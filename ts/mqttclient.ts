import { MisirkaClient } from "./client"
import type { SubscribeToken, MsgHandlerWithTopic } from "./client"
// import type { Resolver, Error } from "./data"
import { random_string } from "./utils"
import { SubscriberTracker } from "./subscr_tracker"
import { CallTracker } from "./call_tracker"
import { get_timeout, call_timeout } from "./utils"

import { Buffer } from "buffer"
import mqtt from "mqtt"
import type { MqttClient, IPublishPacket } from "mqtt"

export interface MQTTClientOpts {
  mqtt_url: string,
  prefix?: string,
  client_id?: string,
  reply_topic_prefix?: string,
  reply_topic_suffix?: string,
  reconnect_period?: number,
  connect_timeout?: number,
  auth_user?: string,
  auth_pass?: string,
  get_timeout?: number,
  call_timeout?: number,
}

export class MQTTClient extends MisirkaClient {
  // FIXME: our class being named MQTTClient while MqttClient exists in "mqtt" is kinda bad
  constructor(
    private opts: MQTTClientOpts,
  ) {
    super()

    const client_id = this.opts.client_id ?? 'misirka_' + random_string(16)
    const reply_topic_prefix = this.opts.reply_topic_prefix ?? `/misirka/call_reply/${client_id}/`
    const reply_topic_suffix = this.opts.reply_topic_suffix ?? random_string(16)
    this.reply_topic = reply_topic_prefix + reply_topic_suffix

    this.conn = mqtt.connect(this.opts.mqtt_url, {
      keepalive: 10,  // seconds
      reschedulePings: true,
      clientId: client_id,
      protocolId: 'MQTT',
      protocolVersion: 5,
      clean: true,  // do not receive QoS1/2 messages while offline
      reconnectPeriod: this.opts.reconnect_period ?? 1000,
      reconnectOnConnackError: true,
      connectTimeout: this.opts.connect_timeout ?? 3000,
      username: this.opts.auth_user,
      password: this.opts.auth_pass,
      autoUseTopicAlias: true,
      autoAssignTopicAlias: true,
      queueQoSZero: false,  // do not queue messages while connection is down
      properties: {
        sessionExpiryInterval: 0, // tell the broker not to resubscribe us to topics
      },
      resubscribe: false, // do not auto-resubscribe
    })

    this.conn.on('connect', () => {
      this.handle_connect()
    })
    this.conn.on('offline', () => {
      this.handle_disconnect()
    })
    this.conn.on('message', (topic, _payload, packet) => {
      this.handle_mqtt_msg(topic, packet)
    })
  }

  private async handle_connect() {
    try {
      await this.conn.subscribeAsync(this.reply_topic)
    } catch (err) {
      console.error(`[misirka mqtt] could not subscribe to call reply topic ${this.reply_topic} and will reconnect: `, err)
      this.conn.reconnect()
      return
    }

    this.notify_alive()
  }

  private handle_disconnect() {
    this.sub_tracker.clear_subscribers()
    this.call_tracker.clear_calls()
    this.notify_dead()
  }

  async subscribe_unsafe(topics: string[], handler: MsgHandlerWithTopic<any>): Promise<Array<SubscribeToken>> {
    return this.sub_tracker.subscribe(topics, handler, async (new_topics) => {
      // TODO: here we should guarantee that we will receive retained messages on the
      // newly-subscribed topics before we return - this might be difficult
      if (new_topics.length == 0) {
        return
      }

      const sub_topics = this.paths(new_topics)
      console.log(`[misirka mqtt] subscribing to topics: ${JSON.stringify(sub_topics)}`)
      await this.conn.subscribeAsync(sub_topics)
    })
  }

  async unsubscribe(tokens: SubscribeToken[]) {
    this.sub_tracker.unsubscribe(tokens, async (removed_topics) => {
      if (removed_topics.length == 0) {
        return
      }

      const unsub_topics = this.paths(removed_topics)
      console.log(`[misirka mqtt] unsubscribing from topics: ${JSON.stringify(unsub_topics)}`)
      await this.conn.unsubscribeAsync(unsub_topics)
    })
  }

  get_unsafe(topic: string, timeout?: number): Promise<any> {
    const msg = this.sub_tracker.get_last_msg(topic)
    if (msg !== undefined) {
      return new Promise((resolve, _) => resolve(msg))
    }

    timeout = get_timeout(this.opts, timeout)

    return new Promise((resolve, reject) => {
      var resolved = false;
      const handler = (_topic: string, msg: any) => {
        if (resolved) {
          return;
        }
        resolved = true;
        resolve(msg);
      }
      this.subscribe_unsafe([topic], handler)
        .then(tok => {
          setTimeout(() => {
            this.unsubscribe(tok).catch(err => {
              console.error(`[misirka mqtt] could not unsubscribe from ${topic} after get timeout: `, err)
            })
            if (!resolved) {
              reject(`request timed out (${timeout}ms)`)
            }
          }, timeout)
        })
        .catch(err => reject(err))
    })
  }

  async call_unsafe(method: string, params: any, timeout?: number): Promise<any> {
    return this.call_tracker.do_call(call_timeout(this.opts, timeout), async (id: number) => {
      await this.send_call_req(method, params, id)
    })
  }

  private async send_call_req(method: string, params: any, id: number) {
    const topic = this.path(method)
    const payload = JSON.stringify(params)
    const opts = {
      qos: 0 as const,
      retain: false,
      properties: {
        payloadFormatIndicator: true,
        responseTopic: this.reply_topic,
        correlationData: Buffer.from(`${id}`),
        contentType: 'application/json',
      },
    }
    await this.conn.publishAsync(topic, payload, opts)
  }

  private handle_reply(msg: IPublishPacket) {
    let payload = JSON.parse(msg.payload.toString('utf-8'))
    let cd = parse_cdata(msg)
    if (!cd) {
      console.error(`[misirka mqtt] received reply message with invalid correlation data`)
    } else if (cd.is_err) {
      this.call_tracker.post_error(cd.id, payload)
    } else {
      this.call_tracker.post_result(cd.id, payload)
    }
  }

  private handle_mqtt_msg(full_topic: string, msg: IPublishPacket) {
    if (full_topic == this.reply_topic) {
      this.handle_reply(msg)
      return
    }

    const topic = this.unpath(full_topic)
    if (topic === undefined) {
      console.error(`[misirka mqtt] received message on topic ${full_topic} which is not under ${this.opts.prefix}`)
      return
    }

    let data: any
    try {
      data = JSON.parse(msg.payload.toString())
    } catch (err) {
      console.error(`[misirka mqtt] could not parse message on topic ${topic}: `, err)
      return
    }
    this.sub_tracker.send_msg(topic, data)
  }

  private paths(ps: string[]): Array<string> {
    return ps.map(p => this.path(p))
  }

  private path(p: string): string {
    return `${this.opts.prefix ?? ""}${p}`
  }

  private unpath(p: string): string | undefined {
    if (!this.opts.prefix) {
      return p
    }

    if (!p.startsWith(this.opts.prefix)) {
      return undefined;
    }

    return p.slice(this.opts.prefix.length)
  }

  private conn: MqttClient
  private reply_topic: string
  private sub_tracker: SubscriberTracker = new SubscriberTracker()
  private call_tracker: CallTracker = new CallTracker()
}

function parse_cdata(msg: IPublishPacket): { id: number, is_err: boolean } | null {
  if (!msg.properties || !msg.properties.correlationData) {
    return null
  }
  const cs = msg.properties.correlationData.toString('utf-8')
  const is_err = cs.endsWith('.error')
  const nonce = cs.split('.')[0]
  const id = Number(nonce)
  if (!Number.isInteger(id) || id.toString() !== nonce) {
    return null;
  }
  return { id: id, is_err: is_err }
}
