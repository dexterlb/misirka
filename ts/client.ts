import type { Schema } from "./schemas"

export interface SubscribeToken {
  topic: string
  id: number
}

export abstract class MisirkaConnTracker {
  // Fired whenever the client connects to the backend.
  // If the client is already connected, the callback fires immediately.
  on_alive(f: () => void): void {
    this.alive_notifiers.push(f)
    if (this.alive()) {
      f();
    }
  }

  // Fired whenever the client disconnects from the backend for whatever
  // reason. All subscriptions done through this client are unsubscribed
  // when this fires, so you will need to subscribe again on connect.
  on_dead(f: () => void): void {
    this.dead_notifiers.push(f)
  }

  alive(): boolean {
    return this.is_alive;
  }

  protected notify_alive() {
    this.is_alive = true;
    for (const f of this.alive_notifiers) {
      f();
    }
  }

  protected notify_dead() {
    this.is_alive = false;
    for (const f of this.dead_notifiers.reverse()) {
      f();
    }
  }

  private is_alive: boolean = false
  private alive_notifiers: Array<() => void> = new Array()
  private dead_notifiers: Array<() => void> = new Array()
}

export type MsgHandlerWithTopic<T> = (topic: string, msg: T) => void
export type MsgHandler<T> = (msg: T) => void

interface RemovedSubscribers {
  removals: Array<[SubscribeToken, MsgHandlerWithTopic<any>]>
  removed_topics: Set<string>
}

export abstract class MisirkaClient extends MisirkaConnTracker {
  async subscribe<T>(topic: string, msg_schema: Schema<T>, handler: MsgHandler<T>): Promise<SubscribeToken> {
    const [sub] = await this.subscribe_multi([topic], msg_schema, (_, msg) => handler(msg));
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

  protected remove_subscribers(tokens: SubscribeToken[]): RemovedSubscribers {
    const removed_topics = new Set<string>()
    const removals = new Array<[SubscribeToken, MsgHandlerWithTopic<any>]>

    for (const token of tokens) {
      const topic_map = this.subscribers.get(token.topic)
      if (topic_map === undefined) {
        continue
      }

      const handler = topic_map.get(token.id)
      if (handler === undefined) {
        continue
      }

      topic_map.delete(token.id)
      removals.push([token, handler])
      if (topic_map.size == 0) {
        this.subscribers.delete(token.topic)
        removed_topics.add(token.topic)
      }
    }

    return {
      removed_topics: removed_topics,
      removals: removals,
    }
  }

  protected undo_remove_subscribers(rs: RemovedSubscribers) {
    for (const [token, handler] of rs.removals) {
      this.add_subscribers([token], handler)
    }
  }

  protected add_subscribers(tokens: SubscribeToken[], handler: MsgHandlerWithTopic<any>): Set<string> {
    const new_topics = new Set<string>()
    for (const token of tokens) {
      let topic_subs = this.subscribers.get(token.topic)
      if (topic_subs === undefined) {
        topic_subs = new Map()
        this.subscribers.set(token.topic, topic_subs)
        new_topics.add(token.topic)
      }
      topic_subs.set(token.id, handler)
    }
    return new_topics
  }

  protected clear_subscribers() {
    this.subscribers = new Map()
  }

  protected async unsubscribe_all() {
    const tokens = new Array<SubscribeToken>
    for (const [topic, subs] of this.subscribers.entries()) {
      for (const id of subs.keys()) {
        tokens.push({
          id: id,
          topic: topic,
        })
      }
    }
    await this.unsubscribe(tokens)
  }

  protected subscribers: Map<string, Map<number, (topic: string, msg: any) => void>> = new Map()

  abstract get_unsafe(topic: string): Promise<any>;
  abstract unsubscribe(tokens: SubscribeToken[]): Promise<void>;
  abstract call_unsafe(method: string, params: any): Promise<any>;
  abstract subscribe_unsafe(topics: string[], handler: MsgHandlerWithTopic<any>): Promise<Array<SubscribeToken>>;
}
