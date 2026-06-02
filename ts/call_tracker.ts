import type { Resolver } from "./data"

export type Caller = (id: number) => Promise<void>

export class CallTracker {
  public new_id(): number {
    const id = this.last_id
    this.last_id++
    return id
  }

  public async do_call(timeout: number, f: Caller): Promise<any> {
    const id = this.new_id()

    await f(id)

    const p = new Promise<any>((resolve, reject) => {
      const timer = setTimeout(() => {
        reject(`Timeout ${timeout}ms (request id ${id})`)
      }, timeout)
      this.resp_handlers.set(id, {
        resolve: (x: any) => {
          clearTimeout(timer)
          resolve(x)
        },
        reject: (x: any) => {
          clearTimeout(timer)
          reject(x)
        },
      })
    })

    try {
      return await p
    } finally {
      this.resp_handlers.delete(id)
    }
  }

  public post_result(id: number, x: any) {
    const handler = this.resp_handlers.get(id)
    if (handler === undefined) {
      console.error(`[misirka] received call result for call ${id} but such a call does not exist`)
      return
    }

    handler.resolve(x)
  }

  public post_error(id: number, err: any) {
    const handler = this.resp_handlers.get(id)
    if (handler === undefined) {
      console.error(`[misirka] received call error for call ${id} but such a call does not exist; error was: `, err)
      return
    }

    handler.reject(err)
  }

  public clear_calls() {
    for (const [_, rh] of this.resp_handlers) {
      rh.reject(new Error('connection died'))
    }
  }

  private resp_handlers: Map<number, Resolver<any>> = new Map()
  private last_id: number = 0
}
