import type { Schema } from "./schemas"

export abstract class MisirkaConnTracker {
  // Fired whenever the client connects to the backend.
  // If the client is already connected, the callback fires immediately.
  on_alive(f: () => void): void {
    this.alive_notifiers.push(f)
    if (this.alive()) {
      f()
    }
  }

  // Fired whenever the client disconnects from the backend for whatever
  // reason. All subscriptions done through this client are unsubscribed
  // when this fires, so you will need to subscribe again on connect.
  on_dead(f: () => void): void {
    this.dead_notifiers.push(f)
  }

  alive(): boolean {
    return this.is_alive
  }

  protected notify_alive() {
    this.is_alive = true
    for (const f of this.alive_notifiers) {
      f()
    }
  }

  protected notify_dead() {
    this.is_alive = false
    for (const f of this.dead_notifiers.reverse()) {
      f()
    }
  }

  private is_alive: boolean = false
  private alive_notifiers: Array<() => void> = []
  private dead_notifiers: Array<() => void> = []
}

export type MsgHandlerWithTopic<T> = (topic: string, msg: T) => void
export type MsgHandler<T> = (msg: T) => void

export interface SubscribeToken {
  topic: string
  id: number
}

export abstract class MisirkaClient extends MisirkaConnTracker {
  async subscribe<T>(topic: string, msg_schema: Schema<T>, handler: MsgHandler<T>): Promise<SubscribeToken> {
    const [sub] = await this.subscribe_multi([topic], msg_schema, (_, msg) => handler(msg))
    return sub
  }

  async subscribe_multi<T>(topics: string[], msg_schema: Schema<T>, handler: MsgHandlerWithTopic<T>): Promise<Array<SubscribeToken>> {
    const raw_handler = (topic: string, raw_msg: any) => {
      let msg: T
      try {
        msg = msg_schema.parse(raw_msg)
      } catch (err) {
        console.error(`received message of wrong type on topic '${topic}': `, err)
        return
      };
      handler(topic, msg)
    }
    return await this.subscribe_unsafe(topics, raw_handler)
  }

  async get<T>(topic: string, schema: Schema<T>): Promise<T> {
    const result = await this.get_unsafe(topic)
    return schema.parse(result)
  }

  async call<T>(method: string, params: any, resp_schema: Schema<T>): Promise<T> {
    const resp = await this.call_unsafe(method, params)
    return resp_schema.parse(resp)
  }

  abstract get_unsafe(topic: string): Promise<any>;
  abstract unsubscribe(tokens: SubscribeToken[]): Promise<void>;
  abstract call_unsafe(method: string, params: any): Promise<any>;
  abstract subscribe_unsafe(topics: string[], handler: MsgHandlerWithTopic<any>): Promise<Array<SubscribeToken>>;
}
