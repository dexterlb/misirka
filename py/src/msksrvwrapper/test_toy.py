from time import localtime, sleep, strftime

from msksrvwrapper import syncserver

class Clock:
    def time_now(self):
        return strftime("%Y-%m-%d %H:%M:%S", localtime())

    def set_timezone(self, tz):
        self._timezone = tz
        return 'ok'

    def main(self):
        srv = syncserver.MskSrv(
            mskpipe_path="/tmp/mskpipe",
            server_settings={
                "http": {"bind": ":8080"},
            },
        )

        srv.open()
        # the doc endpoint needs a server name/description, set before serve()
        srv.set_docs("toy clock", descr="a toy server that publishes the time")
        srv.add_topic(
            "clock", descr="the current time", examples=["2026-07-14 13:42:00"]
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
