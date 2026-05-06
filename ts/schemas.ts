export interface Schema<T> {
  parse: (val: any) => T;
}

export const OkSchema = {
  parse: (x: any): "ok" => {
    if (x !== "ok") {
      throw new Error(`expected "ok" but got ${JSON.stringify(x)}`)
    }
    return "ok"
  }
}

export const BoolSchema = {
  parse: (x: any): boolean => {
    if (x !== true && x !== false) {
      throw new Error(`expected boolean but got ${JSON.stringify(x)}`)
    }
    return x
  }
}
