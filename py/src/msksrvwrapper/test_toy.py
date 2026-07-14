from datetime import datetime
from time import sleep
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from msksrvwrapper import syncserver

class Clock:
    def __init__(self):
        self._zone = ZoneInfo("UTC")

    def time_now(self):
        return datetime.now(self._zone).strftime("%Y-%m-%d %H:%M:%S %Z")

    def set_timezone(self, tz):
        try:
            self._zone = ZoneInfo(tz)
        except (ZoneInfoNotFoundError, ValueError):
            raise ValueError(f"unknown timezone: {tz}")
        return 'ok'

    def main(self):
        srv = syncserver.MskSrv(
            mskpipe_path="/tmp/mskpipe",
            server_settings={
                "http": {"bind": ":8080"},
            },
        )

        srv.open()

        srv.set_docs("toy clock", descr="a toy server that publishes the time")
        srv.add_topic(
            "clock", descr="the current time", examples=["2026-07-14 13:42:00 UTC"]
        )
        srv.add_call_kw(
            "set_timezone",
            handler=self.set_timezone,
            descr="set the current timezone",
            examples=[({'tz': "EEST"}, "ok")],
        )
        srv.serve()

        while True:
            sleep(1)
            srv.publish("clock", self.time_now())

def main():
    Clock().main()

if __name__ == "__main__":
    main()
