export function random_string(n: number) {
  let text = ""
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

  for (let i = 0; i < n; i++) {
    text += chars.charAt(Math.floor(Math.random() * chars.length))
  }

  return text
}
