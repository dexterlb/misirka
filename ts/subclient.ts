import { MisirkaClient } from "./client"
import type { SubscribeToken } from "./client"
import { SubscriberTracker } from "./subscr_tracker"

export interface SubClientOpts {
  prefix: string
  online_topic?: string
  online_filter?: (val: any) => boolean
}

export class SubClient extends MisirkaClient {
  constructor(private base: MisirkaClient, private opts: SubClientOpts) {
    super()
    this.init()
  }

  init() {
    this.base.on_alive(() => {
      if (this.opts.online_topic !== undefined) {
        this.base.subscribe_unsafe([this.opts.online_topic], val => this.handle_online(val))
      } else {
        this.notify_alive()
      }
    })

    this.base.on_dead(() => {
      this.sub_tracker.clear_subscribers()
      this.notify_dead()
    })
  }

  handle_online(val: any) {
    let filter = this.opts.online_filter
    if (filter === undefined) {
      filter = (val) => { if (val) { return true } else { return false } }
    }
    if (filter(val)) {
      this.notify_alive()
    } else {
      this.unsubscribe_all().then(() => {
        this.notify_dead()
      }).catch(err => {
        console.error(`[misirka subclient] Could not unsubscribe successfully after service on ${this.opts.online_topic} died:`, err)
      })
    }
  }

  private async unsubscribe_all() {
    await this.unsubscribe(this.sub_tracker.all_subs())
  }

  async get_unsafe(topic: string): Promise<any> {
    return await this.base.get_unsafe(this.path(topic))
  }

  async unsubscribe(tokens: SubscribeToken[]) {
    await this.base.unsubscribe(tokens)
    this.sub_tracker.remove_subscribers(tokens)
  }

  async call_unsafe(method: string, params: any): Promise<any> {
    return await this.base.call_unsafe(this.path(method), params)
  }

  async subscribe_unsafe(topics: string[], handler: (topic: string, msg: any) => void): Promise<Array<SubscribeToken>> {
    const base_topics = topics.map(topic => this.path(topic))
    const tokens = await this.base.subscribe_unsafe(base_topics, handler)
    this.sub_tracker.add_subscribers(tokens, handler)
    return tokens
  }

  private path(p: string): string {
    return `${this.opts.prefix}${p}`
  }

  private sub_tracker: SubscriberTracker = new SubscriberTracker()
}
