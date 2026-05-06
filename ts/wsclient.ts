import { MisirkaClient } from "./client"
import type { SubscribeToken, MsgHandlerWithTopic } from "./client"
import { OkSchema } from "./schemas"

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

interface Resolver<T> {
  resolve: (val: T) => void;
  reject: (err: any) => void;
}

interface ErrorResult {
  error: Error;
  id?: number;
}

interface Error {
  message: string;
  code: number;
}

export class WSClient extends MisirkaClient {
  constructor(
    private ws_url: string,
  ) {
    super()
    this.init()
  }

  async subscribe_unsafe(topics: string[], handler: MsgHandlerWithTopic<any>): Promise<Array<SubscribeToken>> {
    const tokens: Array<SubscribeToken> = topics.map(topic => { return {
      topic: topic,
      id: this.new_id(),
    }})

    const new_topics = this.add_subscribers(tokens, handler)

    try {
      await this.call("ms-subscribe", Array.from(new_topics), OkSchema)
    } catch (err) {
      // cleanup subscriber handlers in case subscription fails
      this.remove_subscribers(tokens)
      throw err
    }

    return tokens
  }

  async unsubscribe(tokens: SubscribeToken[]) {
    const rem = this.remove_subscribers(tokens)

    try {
      await this.call("ms-unsubscribe", Array.from(rem.removed_topics), OkSchema)
    } catch (err) {
      // oops, we could not unsubscribe
      this.undo_remove_subscribers(rem)
      throw err
    }
  }

  async get_unsafe(topic: string): Promise<any> {
    return await this.call_unsafe('ms-get', topic)
  }

  async call_unsafe(method: string, params: any): Promise<any> {
    const id = this.new_id()
    try {
      const resp = await this.call_raw({
        jsonrpc: "2.0",
        method: method,
        id: id,
        params: params,
      })
      return resp.result
    } finally {
      this.resp_handlers.delete(id)
    }
  }

  private async call_raw(req: Request): Promise<Response> {
    this.ws.send(JSON.stringify(req))

    return new Promise<Response>((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(`Timeout ${req.id}`)
      }, 2000)
      this.resp_handlers.set(req.id, {
        resolve: (x: Response) => {
          clearTimeout(timeout)
          resolve(x)
        },
        reject: (x: any) => {
          clearTimeout(timeout)
          reject(x)
        },
      })
    })
  }

  private init(): void {
    this.ws = new WebSocket(this.ws_url)
    this.resp_handlers = new Map()

    this.ws.onopen = () => {
      this.notify_alive()
    }

    this.ws.onclose = () => {
      setTimeout(() => this.init(), 1000)
      this.clear_subscribers()
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
    const resp_handler = this.pop_resp_handler(resp.id)
    if (resp_handler !== undefined) {
      resp_handler.resolve(resp)
    } else {
      console.error(
        `[misirka wsclient] received response ${JSON.stringify(resp)} but I don't have a matching request`,
      )
    }
  }

  private handle_err(err: ErrorResult) {
    const resp_handler = this.pop_resp_handler(err.id)
    if (resp_handler !== undefined) {
      resp_handler.reject(err.error)
    } else {
      console.error(`[misirka wsclient] received error ${err.error} but I don't have a matching request`)
    }
  }

  private handle_pubmsg(pubmsg: PubMsg) {
    const subscribers = this.subscribers.get(pubmsg.topic)
    if (subscribers === undefined) {
      console.error(`[misirka wsclient] no subscribers for topic ${pubmsg.topic}`)
      return
    }
    for (const sub of subscribers.values()) {
      sub(pubmsg.topic, pubmsg.msg)
    }
  }

  private pop_resp_handler(id: number | undefined): Resolver<Response> | undefined {
    if (id === undefined) {
      return undefined
    }

    const result = this.resp_handlers.get(id)
    if (result === undefined) {
      return undefined
    }
    this.resp_handlers.delete(id)

    return result
  }

  private new_id(): number {
    const id = this.last_id
    this.last_id++
    return id
  }

  private ws!: WebSocket
  private resp_handlers: Map<number, Resolver<Response>> = new Map()
  private last_id: number = 0
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
