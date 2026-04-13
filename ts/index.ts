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

interface Schema<T> {
  parse: (val: any) => T
}

export class WSClient {
  constructor(
    private ws_url: string,
    private on_connect: () => void,
    private on_disconnect: () => void,
  ) {
    this.init();
  }

  private init(): void {
    this.ws = new WebSocket(this.ws_url);
    this.resp_handlers = new Map();
    this.subscribers = new Map();

    this.ws.onopen = () => {
      console.log("WebSocket connected!");
      this.on_connect();
    };

    this.ws.onclose = () => {
      console.log("WebSocket disconnected, trying to reconnect");
      setTimeout(() => this.init(), 1000);
      this.on_disconnect();
    };

    this.ws.onerror = (err) => console.error("WebSocket error:", err);

    this.ws.onmessage = (event) => {
      this.handle_msg(event.data);
    };
  }

  private handle_msg(data: string) {
    const resp = parseResponse(data);
    if (resp !== null) {
      this.handle_resp(resp);
      return;
    }

    const err = parseError(data);
    if (err !== null) {
      this.handle_err(err);
      return;
    }

    const pubmsg = parsePubMsg(data);
    if (pubmsg !== null) {
      this.handle_pubmsg(pubmsg);
      return;
    }

    console.error(
      `received unwanted data on websocket: ${data}`
    );
  }

  private handle_resp(resp: Response) {
    const resp_handler = this.pop_resp_handler(resp.id);
    if (resp_handler !== undefined) {
      resp_handler.resolve(resp);
    } else {
      console.error(
        `received response ${resp} but I don't have a matching request`,
      );
    }
  }

  private handle_err(err: ErrorResult) {
    const resp_handler = this.pop_resp_handler(err.id);
    if (resp_handler !== undefined) {
      resp_handler.reject(err.error);
    } else {
      console.error(`received error ${err.error} but I don't have a matching request`);
    }
  }

  private handle_pubmsg(pubmsg: PubMsg) {
    const subscribers = this.subscribers.get(pubmsg.topic);
    if (subscribers === undefined) {
      console.error(`no subscribers for topic ${pubmsg.topic}`);
      return;
    }
    for (const sub of subscribers) {
      sub(pubmsg.topic, pubmsg.msg);
    }
  }

  async subscribe<T>(topics: string[], msg_schema: Schema<T>, handler: (msg: T) => void) {
    const raw_handler = (topic: string, raw_msg: any) => {
      var msg: T;
      try {
        msg = msg_schema.parse(raw_msg);
      } catch (err) {
        console.error(`received message of wrong type on topic '${topic}': `, err);
        return;
      };
      handler(msg);
    };
    await this.subscribe_unsafe(topics, raw_handler);
  }

  async subscribe_unsafe(topics: string[], handler: (topic: string, msg: any) => void) {
    for (const topic of topics) {
      var topic_subs = this.subscribers.get(topic);
      if (topic_subs === undefined) {
        topic_subs = []
        this.subscribers.set(topic, topic_subs);
      }
      topic_subs.push(handler);
    }

    try {
      const subscribe_resp = await this.request_unsafe("ms-subscribe", topics);
      if (subscribe_resp !== "ok") {
        throw new Error(`subscribe call returned '${JSON.stringify(subscribe_resp)}' instead of 'ok'`);
      }
    } catch (err) {
        // cleanup subscriber handlers in case subscription fails
        for (const topic of topics) {
          const topic_subs = this.subscribers.get(topic);
          if (topic_subs !== undefined) {
            topic_subs.pop();
          }
        }
        throw err;
    }
  }

  async get<T>(topic: string, schema: Schema<T>): Promise<T> {
    return await this.request('ms-get', topic, schema);
  }

  async get_unsafe(topic: string): Promise<any> {
    return await this.request_unsafe('ms-get', topic);
  }

  async request<T>(method: string, params: any, resp_schema: Schema<T>): Promise<T> {
    const resp = await this.request_unsafe(method, params);
    return resp_schema.parse(resp);
  }

  async request_unsafe(method: string, params: any): Promise<any> {
    const id = this.last_id;
    this.last_id++;
    const resp = await this.request_raw({
      jsonrpc: "2.0",
      method: method,
      id: id,
      params: params,
    });

    return resp.result;
  }

  private async request_raw(req: Request): Promise<Response> {
    this.ws.send(JSON.stringify(req));

    return new Promise<Response>((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(`Timeout ${req.id}`);
      }, 2000);
      this.resp_handlers.set(req.id, {
        resolve: (x: Response) => {
          clearTimeout(timeout);
          resolve(x);
        },
        reject: (x: any) => {
          clearTimeout(timeout);
          reject(x);
        },
      });
    });
  }

  private pop_resp_handler(id: number | undefined): Resolver<Response> | undefined {
    if (id === undefined) {
      return undefined;
    }

    const result = this.resp_handlers.get(id);
    if (result === undefined) {
      return undefined;
    }
    this.resp_handlers.delete(id);

    return result;
  }

  private ws!: WebSocket;
  private resp_handlers: Map<number, Resolver<Response>> = new Map();
  private last_id: number = 0;
  private subscribers: Map<string, Array<(topic: string, msg: any) => void>> = new Map();
}

function parseResponse(data: string): Response | null {
  const parsed = JSON.parse(data);

  if ("result" in parsed && "id" in parsed) {
    return parsed as Response;
  }

  return null;
}

function parseError(data: string): ErrorResult | null {
  const parsed = JSON.parse(data);

  if (
    "error" in parsed &&
    "message" in parsed["error"] &&
    "code" in parsed["error"]
  ) {
    return parsed as ErrorResult;
  }

  return null;
}

function parsePubMsg(data: string): PubMsg | null {
  const parsed = JSON.parse(data);

  if ("topic" in parsed && "msg" in parsed) {
    return parsed as PubMsg;
  }

  return null;
}
