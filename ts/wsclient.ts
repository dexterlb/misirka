import { MisirkaClient } from "./client"
import type { SubscribeToken, MsgHandlerWithTopic } from "./client"
import type { Error } from "./data"
import { OkSchema } from "./schemas"
import { SubscriberTracker } from "./subscr_tracker"
import { CallTracker } from "./call_tracker"
import { get_timeout, call_timeout } from "./utils"

interface Request {
  jsonrpc: "2.0";
  method: string;
  params: any;
  id: number;
}

interface Response {
  result: any;
  id: number;
}

interface PubMsg {
  topic: string;
  msg: any;
}

interface ErrorResult {
  error: Error;
  id?: number;
}

export interface WSClientOpts {
  ws_url: string,
  get_timeout?: number,
  call_timeout?: number,
  reconnect_period?: number,
}

export class WSClient extends MisirkaClient {
  constructor(
    private opts: WSClientOpts,
  ) {
    super()
    this.init()
  }

  async subscribe_unsafe(topics: string[], handler: MsgHandlerWithTopic<any>): Promise<Array<SubscribeToken>> {
    return this.sub_tracker.subscribe(topics, handler, async (new_topics) => {
      // ms-subscribe must guarantee that it will first send messages on the
      // newly-subscribed topics and then return
      await this.call("ms-subscribe", new_topics, OkSchema)
    })
  }

  async unsubscribe(tokens: SubscribeToken[]) {
    this.sub_tracker.unsubscribe(tokens, async (removed_topics) => {
      await this.call("ms-unsubscribe", removed_topics, OkSchema)
    })
  }

  async get_unsafe(topic: string, timeout?: number): Promise<any> {
    return await this.call_unsafe('ms-get', topic, get_timeout(this.opts, timeout))
  }

  async call_unsafe(method: string, params: any, timeout?: number): Promise<any> {
    return this.call_tracker.do_call(call_timeout(this.opts, timeout), async (id) => {
      const req: Request = {
        jsonrpc: "2.0",
        method: method,
        id: id,
        params: params,
      }
      this.ws.send(JSON.stringify(req))
    })
  }

  private init(): void {
    this.ws = new WebSocket(this.opts.ws_url)

    this.ws.onopen = () => {
      this.notify_alive()
    }

    this.ws.onclose = () => {
      setTimeout(() => this.init(), this.opts.reconnect_period ?? 1000)
      this.sub_tracker.clear_subscribers()
      this.call_tracker.clear_calls()
      this.notify_dead()
    }

    this.ws.onerror = (err) => console.error("[misirka wsclient] WebSocket error:", err)

    this.ws.onmessage = (event) => {
      this.handle_msg(event.data)
    }
  }

  private handle_msg(data: string) {
    const resp = parseResponse(data)
    if (resp !== null) {
      this.handle_resp(resp)
      return
    }

    const err = parseError(data)
    if (err !== null) {
      this.handle_err(err)
      return
    }

    const pubmsg = parsePubMsg(data)
    if (pubmsg !== null) {
      this.handle_pubmsg(pubmsg)
      return
    }

    console.error(
      `[misirka wsclient] received unwanted data on websocket: ${data}`,
    )
  }

  private handle_resp(resp: Response) {
    this.call_tracker.post_result(resp.id, resp.result)
  }

  private handle_err(err: ErrorResult) {
    if (err.id === undefined) {
      console.error(
        '[misirka wsclient] received generic error on websocket: ',
        err.error
      )
      return
    }
    this.call_tracker.post_error(err.id, err.error)
  }

  private handle_pubmsg(pubmsg: PubMsg) {
    this.sub_tracker.send_msg(pubmsg.topic, pubmsg.msg)
  }

  private ws!: WebSocket
  private sub_tracker: SubscriberTracker = new SubscriberTracker()
  private call_tracker: CallTracker = new CallTracker()
}

function parseResponse(data: string): Response | null {
  const parsed = JSON.parse(data)

  if ("result" in parsed && "id" in parsed) {
    return parsed as Response
  }

  return null
}

function parseError(data: string): ErrorResult | null {
  const parsed = JSON.parse(data)

  if (
    "error" in parsed &&
    "message" in parsed["error"] &&
    "code" in parsed["error"]
  ) {
    return parsed as ErrorResult
  }

  return null
}

function parsePubMsg(data: string): PubMsg | null {
  const parsed = JSON.parse(data)

  if ("topic" in parsed && "msg" in parsed) {
    return parsed as PubMsg
  }

  return null
}
