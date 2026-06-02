export function random_string(n: number) {
  let text = ""
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

  for (let i = 0; i < n; i++) {
    text += chars.charAt(Math.floor(Math.random() * chars.length))
  }

  return text
}

export function get_timeout(opts: { get_timeout?: number, call_timeout?: number }, timeout?: number) {
    return timeout ?? opts.get_timeout ?? opts.call_timeout ?? 3000
}

export function call_timeout(opts: { call_timeout?: number }, timeout?: number) {
    return timeout ?? opts.call_timeout ?? 5000
}
