import json
import subprocess
import sys
import threading
from itertools import count
from typing import Optional


class _Pending:
    def __init__(self):
        self.event = threading.Event()
        self.response: Optional[dict] = None

    def respond(self, response: dict):
        self.response = response
        self.event.set()


class MskSrv:
    def __init__(self, mskpipe_path, server_settings):
        self._mskpipe_path = mskpipe_path
        self._server_settings = server_settings

        self._proc = None   # child mskpipe process
        self._reader = None # reader thread

        self._reqs = {}
        self._reqs_lock = threading.Lock()

        self._call_handlers = {}

        self._stdin_lock = threading.Lock()

        self._ready = threading.Event()

        self._ids = count(1)

    def open(self):
        """Launch the pipe subprocess and initialise the server."""
        self._reader = threading.Thread(target=self._pump, daemon=True)
        self._reader.start()
        self._ready.wait()
        self._req("init", self._server_settings)

    def set_docs(self, name, descr=""):
        return self._req("set_docs", {"name": name, "descr": descr})

    def add_call_kw(self, path, handler, descr="", examples=None):
        param_handler = lambda d: handler(**d)
        self.add_call(path, param_handler, descr, examples)

    def add_call(self, path, handler, descr="", examples=None):
        self._call_handlers[path] = handler
        return self._req(
            "add_call",
            {"path": path, "descr": descr, "examples": examples or []},
        )

    def add_topic(self, path, descr, examples):
        return self._req(
            "add_topic",
            {"path": path, "descr": descr, "examples": examples or []},
        )

    def publish(self, path, data):
        try:
            self.publish_or_die(path, data)
        except:
            print(
                f"mskpipe: failed to publish on {path}",
                file=sys.stderr,
            )

    def publish_or_die(self, path, data):
        return self._req("publish", {"path": path, "data": data})

    def serve(self):
        # no need to wait for the result of this request, because it only
        # returns if there has been an error, and that would cause the
        # subprocess to exit anyway
        self._submit_req(
            {"method": "serve", "params": {}, "id": 0}
        )

    def close(self):
        """Terminate the pipe subprocess."""
        if self._proc is not None:
            self._proc.terminate()

    def _req(self, method, params = None) -> dict:
        return self._raw_req(
            {"method": method, "params": params, "id": next(self._ids)}
        )

    def _raw_req(self, rpc_req):
        req_id = rpc_req["id"]
        pending = _Pending()
        with self._reqs_lock:
            self._reqs[req_id] = pending

        try:
            self._submit_req(rpc_req)

            pending.event.wait()
            return pending.response
        finally:
            with self._reqs_lock:
                self._reqs.pop(req_id, None)

    def _submit_req(self, rpc_req):
        line = json.dumps(rpc_req)
        with self._stdin_lock:
            if self._proc is None or self._proc.stdin is None:
                raise RuntimeError("mskpipe process is not running")
            self._proc.stdin.write(line + "\n")
            self._proc.stdin.flush()

    def _pump(self):
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

        self._ready.set()

        for line in self._proc.stdout:
            line = line.strip()
            if line:
                self._handle_stdout_line(line)

        self._proc.wait()
        self._fail_pending()

    def _respond(self, req_id, response):
        with self._reqs_lock:
            pending = self._reqs.pop(req_id, None)
        if pending is None:
            print(
                f"mskpipe: response for unknown req_id {req_id}: {response}",
                file=sys.stderr,
            )
            return
        pending.respond(response)

    def _handle_stdout_line(self, line: str):
        try:
            msg = json.loads(line)
        except json.JSONDecodeError:
            print(f"mskpipe: could not parse stdout line: {line}", file=sys.stderr)
            return

        if 'method' in msg and 'id' in msg:
            self._handle_call(msg['id'], msg['method'], msg['params'])
            return

        if 'result' in msg and 'id' in msg:
            self._respond(msg['id'], msg['result'])
            return

        print(f"mskpipe: don't know what to do with this: {json.dumps(msg)}", file=sys.stderr)

    def _handle_call(self, req_id, path, param):
        handler = self._call_handlers.get(path)

        if handler is None:
            self._reply_call(req_id, error=f"no handler for call {path}")
            return

        try:
            result = handler(param)
        except Exception as e:
            self._reply_call(req_id, error=str(e))
            return

        self._reply_call(req_id, result=result)

    def _reply_call(self, req_id, result=None, error=None):
        if error is not None:
            resp = {"id": req_id, "error": {"code": -37000, "message": error}}
        else:
            resp = {"id": req_id, "result": result}
        self._submit_req(resp)


    def _forward_stderr(self):
        assert self._proc is not None and self._proc.stderr is not None
        for line in self._proc.stderr:
            print(
                f"mskpipe: {line.strip()}",
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
