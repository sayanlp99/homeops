// Cloudflare Email Worker: hands mail for your domain to the home mail server.
// Runs on the Workers Free plan (100k requests/day); no paid plan is needed.
//
// Email Routing receives the message (Cloudflare runs the MX), this worker POSTs it
// through the Cloudflare Tunnel to mailbridge, which delivers it to the local mailbox.
// If the home server is down, the message is forwarded to FALLBACK_TO instead, so
// nothing is lost.
//
// Worker settings (Settings → Variables and Secrets):
//   BRIDGE_URL     https://mailin.example.com/inbound
//   BRIDGE_TOKEN   (secret) the MAILBRIDGE_TOKEN from the server's /etc/homeops/mailbridge.env
//   FALLBACK_TO    a verified Email Routing destination, e.g. you@gmail.com (optional)

export default {
  async email(message, env) {
    // Stream the message straight through: no buffering, so even a 25 MiB message
    // stays well inside the Workers Free plan's CPU and memory limits.
    let res
    try {
      res = await fetch(env.BRIDGE_URL, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${env.BRIDGE_TOKEN}`,
          'Content-Type': 'message/rfc822',
          'X-Envelope-From': message.from || '',
          'X-Envelope-To': message.to,
          'Content-Length': String(message.rawSize),
        },
        body: message.raw,
      })
    } catch (err) {
      res = { ok: false, status: 0, text: async () => String(err) }
    }
    if (res.ok) return

    const reason = (await res.text()).slice(0, 200)
    // The server looked and said no (unknown mailbox, policy): bounce like any mail server.
    if (res.status === 422) {
      message.setReject(reason || 'Mailbox unavailable')
      return
    }
    // Anything else is temporary: keep the message by forwarding it.
    console.log(`bridge unavailable (${res.status}): ${reason}`)
    if (env.FALLBACK_TO) {
      await message.forward(env.FALLBACK_TO, new Headers({ 'X-HomeOps-Fallback': `bridge ${res.status}` }))
      return
    }
    message.setReject('Temporarily unable to deliver, please retry later')
  },
}
