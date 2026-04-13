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
      const resp_handler = this.pop_resp_handler(resp.id);
      if (resp_handler !== undefined) {
        resp_handler.resolve(resp);
      } else {
        console.error(
          `received response ${resp} but I don't have a matching request`,
        );
      }
      return;
    }

    const err = parseError(data);
    if (err !== null) {
      const resp_handler = this.pop_resp_handler(err.id);
      if (resp_handler !== undefined) {
        resp_handler.reject(err.error);
      } else {
        console.error(`received error ${err.error} but I don't have a matching request`);
      }
      return;
    }

    const pubmsg = parsePubMsg(data);
    if (pubmsg !== null) {
      throw new Error("not implemented");
      return;
    }
  }

  async request(method: string, params: any): Promise<any> {
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
