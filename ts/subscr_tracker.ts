import type { MsgHandlerWithTopic, SubscribeToken } from "./client"

export interface RemovedSubscribers {
  removals: Array<[SubscribeToken, MsgHandlerWithTopic<any>]>
  removed_topics: Set<string>
}

export type TopicSubscriber = (topics: Array<string>) => Promise<void>

export class SubscriberTracker {
  constructor() {
  }
  // Add subscription handlers for the given topics and call f
  // for newly-subscribed topics
  public async subscribe(topics: string[], handler: MsgHandlerWithTopic<any>, f: TopicSubscriber): Promise<Array<SubscribeToken>> {
    const tokens: Array<SubscribeToken> = topics.map(topic => { return {
      topic: topic,
      id: this.new_id(),
    }})

    const new_topics = this.add_subscribers(tokens, handler)

    const gen = this.generation

    try {
      this.add_dirty_topics(new_topics)
      await f(Array.from(new_topics.values()))

      if (gen != this.generation) {
        throw new Error('subscribers were cleared (connection died?) while trying to subscribe')
      }
    } catch (err) {
      // cleanup subscriber handlers in case subscription fails
      this.remove_subscribers(tokens)
      throw err
    } finally {
      this.remove_dirty_topics(new_topics)
    }

    return tokens
  }

  // Remove the subscription handlers associated with the given tokens,
  // and then call f with the topics that don't have subscribers anymore
  public async unsubscribe(tokens: SubscribeToken[], f: TopicSubscriber) {
    const rem = this.remove_subscribers(tokens)

    const gen = this.generation

    try {
      this.add_dirty_topics(rem.removed_topics)
      await f(Array.from(rem.removed_topics.values()))
    } catch (err) {
      // oops, we could not unsubscribe
      if (gen != this.generation) {
        // subscribers were cleared anyway, no need to undo
        return;
      }
      this.undo_remove_subscribers(rem)

      throw err
    } finally {
      this.remove_dirty_topics(rem.removed_topics)
    }
  }

  public add_subscribers(tokens: SubscribeToken[], handler: MsgHandlerWithTopic<any>): Set<string> {
    const new_topics = new Set<string>()
    for (const token of tokens) {
      let topic_subs = this.subscribers.get(token.topic)
      if (topic_subs === undefined) {
        topic_subs = new Map()
        this.subscribers.set(token.topic, topic_subs)
        new_topics.add(token.topic)
      } else {
        const msg = this.last_msg.get(token.topic)
        if (msg === undefined) {
          console.error(`[misirka] topic ${token.topic} has been subscribed some time ago but we haven't received any message on it?`)
        } else {
          handler(token.topic, msg)
        }
      }
      topic_subs.set(token.id, handler)
    }
    return new_topics
  }

  public clear_subscribers() {
    this.generation++
    this.subscribers = new Map()
  }

  public all_subs(): Array<SubscribeToken> {
    const tokens = new Array<SubscribeToken>
    for (const [topic, subs] of this.subscribers.entries()) {
      for (const id of subs.keys()) {
        tokens.push({
          id: id,
          topic: topic,
        })
      }
    }
    return tokens
  }

  public remove_subscribers(tokens: SubscribeToken[]): RemovedSubscribers {
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
        this.last_msg.delete(token.topic)
        removed_topics.add(token.topic)
      }
    }

    return {
      removed_topics: removed_topics,
      removals: removals,
    }
  }

  public undo_remove_subscribers(rs: RemovedSubscribers) {
    for (const [token, handler] of rs.removals) {
      this.add_subscribers([token], handler)
    }
  }

  public send_msg(topic: string, msg: any) {
    this.last_msg.set(topic, msg)
    const subscribers = this.subscribers.get(topic)
    if (subscribers === undefined) {
      console.error(`[misirka] no subscribers for topic ${topic}`)
      return
    }
    for (const sub of subscribers.values()) {
      try {
        sub(topic, msg)
      } catch (err) {
        console.error(`[misirka] exception while handling message on topic ${topic}: `, err)
      }
    }
  }

  public get_last_msg(topic: string): any | undefined {
    return this.last_msg.get(topic)
  }

  private new_id(): number {
    const id = this.last_id
    this.last_id++
    return id
  }

  private add_dirty_topics(topics: Set<string>) {
    const common = this.dirty_topics.intersection(topics)
    if (common.size != 0) {
      throw new Error(
        `concurrent subscribe/unsubscribe for the same topic is currently unsupported, but you tried it for these topics: ${JSON.stringify(common)}`
      )
    }

    for (const topic of topics) {
      this.dirty_topics.add(topic)
    }
  }

  private remove_dirty_topics(topics: Set<string>) {
    for (const topic of topics) {
      this.dirty_topics.delete(topic)
    }
  }

  private dirty_topics: Set<string> = new Set()
  private subscribers: Map<string, Map<number, MsgHandlerWithTopic<any>>> = new Map()
  private last_msg: Map<string, any> = new Map()
  private last_id: number = 0
  private generation: number = 0
}
