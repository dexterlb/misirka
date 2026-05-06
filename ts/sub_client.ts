import { MisirkaClient } from "./client"
import type { SubscribeToken } from "./client"

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
      this.clear_subscribers()
      this.notify_dead()
    })
  }

  handle_online(val: any) {
    var filter = this.opts.online_filter
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

  async get_unsafe(topic: string): Promise<any> {
    return await this.base.get_unsafe(this.path(topic))
  }

  async unsubscribe(tokens: SubscribeToken[]) {
    await this.base.unsubscribe(tokens)
    this.remove_subscribers(tokens)
  }

  async call_unsafe(method: string, params: any): Promise<any> {
    return await this.base.call_unsafe(this.path(method), params)
  }

  async subscribe_unsafe(topics: string[], handler: (topic: string, msg: any) => void): Promise<Array<SubscribeToken>> {
    const tokens = await this.base.subscribe_unsafe(topics, handler)
    this.add_subscribers(tokens, handler)
    return tokens
  }

  private path(p: string): string {
    return `${this.opts.prefix}${p}`
  }
}
