import { MisirkaClient } from "./client"
import type { SubscribeToken, MsgHandlerWithTopic } from "./client"
// import type { Resolver, Error } from "./data"
import { random_string } from "./utils"
import { SubscriberTracker } from "./subscr_tracker"

import mqtt from "mqtt"
import type { MqttClient, IPublishPacket } from "mqtt"

export interface MQTTClientOpts {
  mqtt_url: string,
  prefix?: string,
  client_id?: string,
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

    this.conn = mqtt.connect(this.opts.mqtt_url, {
      keepalive: 10,  // seconds
      reschedulePings: true,
      clientId: this.opts.client_id ?? 'misirka_' + random_string(16),
      protocolId: 'MQTT',
      protocolVersion: 5,
      clean: true,  // do not receive QoS1/2 messages while offline
      reconnectPeriod: this.opts.reconnect_period ?? 1000,
      reconnectOnConnackError: true,
      connectTimeout: this.opts.connect_timeout ?? 3000,
      username: this.opts.auth_user,
      password: this.opts.auth_pass,
      queueQoSZero: false,  // do not queue messages while connection is down
      properties: {
        sessionExpiryInterval: 0, // tell the broker not to resubscribe us to topics
      },
      resubscribe: false, // do not auto-resubscribe
    })
    this.conn.on('connect', () => {
      this.notify_alive()
    })
    this.conn.on('message', (topic, _payload, packet) => {
      this.handle_mqtt_msg(topic, packet)
    })
  }

  async subscribe_unsafe(topics: string[], handler: MsgHandlerWithTopic<any>): Promise<Array<SubscribeToken>> {
    return this.sub_tracker.subscribe(topics, handler, async (new_topics) => {
      // TODO: here we must guarantee that we will receive first messages on the
      // newly-subscribed topics and then return - this might be difficult
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

  get_unsafe(topic: string): Promise<any> {
    const msg = this.sub_tracker.get_last_msg(topic)
    if (msg !== undefined) {
      return new Promise((resolve, _) => resolve(msg))
    }

    const timeout = this.opts.get_timeout ?? this.opts.call_timeout ?? 3000

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

  async call_unsafe(_method: string, _params: any): Promise<any> {
    // FIXME: implement me
  }

  private handle_mqtt_msg(full_topic: string, msg: IPublishPacket) {
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
  private sub_tracker: SubscriberTracker = new SubscriberTracker()
}
