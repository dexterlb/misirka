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
      const sub_topics = this.paths(new_topics)
      console.log(`[misirka mqtt] subscribing to topics: ${JSON.stringify(sub_topics)}`)
      await this.conn.subscribeAsync(sub_topics)
    })
  }

  async unsubscribe(tokens: SubscribeToken[]) {
    this.sub_tracker.unsubscribe(tokens, async (removed_topics) => {
      const unsub_topics = this.paths(removed_topics)
      console.log(`[misirka mqtt] unsubscribing from topics: ${JSON.stringify(unsub_topics)}`)
      await this.conn.unsubscribeAsync(unsub_topics)
    })
  }

  async get_unsafe(_topic: string): Promise<any> {
    // FIXME: implement me:
    // - subscribe to topic with a handler that resolves the promise
    // - set a timeout that unsubscribes the topic and rejects the promise if it is not
    //   already resolved
    // this way we will keep the topic subscribed for a while
  }

  async call_unsafe(_method: string, _params: any): Promise<any> {
    // FIXME: implement me
  }

  private handle_mqtt_msg(topic: string, msg: IPublishPacket) {
    console.log(`got msg on topic ${topic}: ${msg.payload}`)
  }

  private paths(ps: string[]): Array<string> {
    return ps.map(p => this.path(p))
  }

  private path(p: string): string {
    return `${this.opts.prefix ?? ""}${p}`
  }

  private conn: MqttClient
  private sub_tracker: SubscriberTracker = new SubscriberTracker()
}
