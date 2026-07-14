import json
import subprocess
import sys
import threading
from itertools import count
from typing import Any, Optional


class _Pending:
    def __init__(self):
        self.event = threading.Event()
        self.response: Optional[dict] = None

    def respond(self, response: dict):
        self.response = response
        self.event.set()


class MskSrv(threading.Thread):
    def __init__(self, mskpipe_path: str, server_settings: dict()):
        super().__init__(daemon=True)
        self._mskpipe_path = mskpipe_path
        self._server_settings = server_settings

        self._proc: Optional[subprocess.Popen] = None
        # req_id -> _Pending, guarded by _reqs_lock
        self._reqs: dict[int, _Pending] = {}
        self._reqs_lock = threading.Lock()
        # serialises writes to the subprocess' stdin
        self._stdin_lock = threading.Lock()
        self._ids = count(1)

    def _req(self, method: str, params: Any = None) -> dict:
        return self._raw_req(
            {"method": method, "params": params, "id": next(self._ids)}
        )

    def _raw_req(self, rpc_req: dict) -> dict:
        req_id = rpc_req["id"]
        pending = _Pending()
        with self._reqs_lock:
            self._reqs[req_id] = pending

        try:
            line = json.dumps(rpc_req)
            with self._stdin_lock:
                if self._proc is None or self._proc.stdin is None:
                    raise RuntimeError("mskpipe process is not running")
                self._proc.stdin.write(line + "\n")
                self._proc.stdin.flush()

            pending.event.wait()
            return pending.response
        finally:
            with self._reqs_lock:
                self._reqs.pop(req_id, None)

    def run(self):
        self._proc = subprocess.Popen(
            [self._mskpipe_path],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1,  # line-buffered
        )

        stderr_thread = threading.Thread(target=self._forward_stderr, daemon=True)
        stderr_thread.start()

        for line in self._proc.stdout:
            line = line.strip()
            if line:
                self._handle_stdout_line(line)

        # stdout closed => the process is exiting; unblock anyone still waiting.
        self._proc.wait()
        self._fail_pending()

    def _respond(self, req_id, response):
        with self._reqs_lock:
            pending = self._reqs.pop(req_id, None)
        if pending is None:
            print(
                f"mskpipe: response for unknown req_id {req_id!r}: {line}",
                file=sys.stderr,
            )
            return
        pending.respond(response)

    def _handle_stdout_line(self, line: str):
        try:
            msg = json.loads(line)
        except json.JSONDecodeError:
            print(f"mskpipe: could not parse stdout line: {line!r}", file=sys.stderr)
            return

        if 'result' in msg and 'id' in msg:
            self._respond(msg['id'], msg['result'])
            return

        print(f"mskpipe: don't know what to do with this: {json.dumps(msg)}", file=sys.stderr)


    def _forward_stderr(self):
        assert self._proc is not None and self._proc.stderr is not None
        for line in self._proc.stderr:
            print(
                f"mskpipe: {line}",
                file=sys.stderr,
            )
        sys.stderr.flush()

    def _fail_pending(self):
        with self._reqs_lock:
            pendings = list(self._reqs.items())
            self._reqs.clear()

        for req_id, pending in pendings:
            pending.respond(
                {
                    "id": req_id,
                    "error": {"message": "mskpipe process exited"},
                    "jsonrpc": "2.0",
                }
            )
