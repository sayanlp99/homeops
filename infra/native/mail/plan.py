#!/usr/bin/env python3
"""Render the Stalwart apply plan (NDJSON) for the HomeOps mail setup.

Everything comes from the environment, so secrets never live in the repo:
  MAIL_DOMAIN       sayan.fit
  MAIL_BIND         0.0.0.0           where IMAP/submission listen (firewall limits to LAN/ZeroTier)
  PORT_BRIDGE/PORT_SUBMISSION/PORT_IMAPS/PORT_HTTP   default 2525/587/993/8480
  MAILBOXES         "me:Sayan"        space-separated local-part[:Full Name]
  ALIASES           "postmaster:me"   alias:target pairs
  APP_SENDER        no-reply          account HomeOps and homeguard send as
  PASS_<local>      password for each mailbox (e.g. PASS_me, PASS_no_reply)
  BREVO_LOGIN       Brevo SMTP login; when set, all outbound mail relays via Brevo
                    (the key is read by Stalwart from BREVO_SMTP_KEY in its env file)
"""
import json, os, sys

env = os.environ.get
domain = env("MAIL_DOMAIN", "sayan.fit")
bind = env("MAIL_BIND", "0.0.0.0")
ports = {k: int(env("PORT_" + k.upper(), d)) for k, d in
         [("bridge", "2525"), ("submission", "587"), ("imaps", "993"), ("http", "8480")]}
out = []
op = lambda **o: out.append(json.dumps(o, separators=(",", ":")))
# JMAP encoding: lists are {"0": item, ...}; sets are {value: true, ...}.
listish = lambda xs: {str(i): x for i, x in enumerate(xs)}
setish = lambda xs: {x: True for x in xs}

def listener(name, protocol, addrs, tls=False, implicit=False):
    return {"name": name, "protocol": protocol, "bind": setish(addrs), "useTls": tls, "tlsImplicit": implicit}

op(**{"@type": "upsert", "object": "NetworkListener", "matchOn": ["name"], "value": {
    # Mail from the Cloudflare Email Worker arrives through the local bridge only.
    "smtp-bridge": listener("smtp-bridge", "smtp", [f"127.0.0.1:{ports['bridge']}"]),
    "submission": listener("submission", "smtp", [f"{bind}:{ports['submission']}"], tls=True),
    "imaps": listener("imaps", "imap", [f"{bind}:{ports['imaps']}"], tls=True, implicit=True),
    # Filters and vacation replies from webmail.
    "sieve": listener("sieve", "manageSieve", ["127.0.0.1:4190"]),
    # Admin WebUI + JMAP: loopback only (reach it with an SSH tunnel).
    "http": listener("http", "http", [f"127.0.0.1:{ports['http']}"]),
}})
# Inbound mail from the bridge (loopback only) is like port 25: no sign-in needed to
# deliver to local mailboxes. Relaying elsewhere still requires authentication.
op(**{"@type": "update", "object": "MtaStageAuth", "value": {"require": {
    "match": {"0": {"if": "listener == 'smtp-bridge'", "then": "false"}}, "else": "local_port != 25"}}})
# Bridged mail always connects from 127.0.0.1, so SPF and reverse-IP are meaningless there,
# but DKIM, DMARC (which can pass on DKIM alone) and Cloudflare's ARC seal still tell
# genuine mail apart. "relaxed" scores failures instead of rejecting.
bridge_or_25 = lambda then: {"match": {"0": {"if": "listener == 'smtp-bridge'", "then": then},
                                       "1": {"if": "local_port == 25", "then": "relaxed"}}, "else": "disable"}
op(**{"@type": "update", "object": "SenderAuth", "value": {
    "dmarcVerify": bridge_or_25("relaxed"), "arcVerify": bridge_or_25("relaxed")}})
op(**{"@type": "upsert", "object": "Domain", "matchOn": ["name"], "value": {"dom": {"name": domain}}})

def pw(local):
    v = env("PASS_" + local.replace("-", "_").replace(".", "_"))
    if not v:
        sys.exit(f"missing PASS_{local.replace('-', '_')}")
    return {"0": {"@type": "Password", "secret": v}}

accounts = {}
mailboxes = [m.split(":", 1) for m in env("MAILBOXES", "me:Me").split()]
aliases = [a.split(":", 1) for a in env("ALIASES", "").split()]
for local, *name in mailboxes:
    al = [{"name": a, "domainId": "#dom"} for a, target in aliases if target == local]
    accounts[local] = {"@type": "User", "name": local, "domainId": "#dom", "description": name[0] if name else local,
                       "credentials": pw(local), "aliases": listish(al)}
sender = env("APP_SENDER", "no-reply")
accounts[sender] = {"@type": "User", "name": sender, "domainId": "#dom", "description": "HomeOps notifications",
                    "credentials": pw(sender), "aliases": {}}
if env("PASS_admin"):  # the administrator created during setup; keep its password in our secrets file
    accounts["admin"] = {"@type": "User", "name": "admin", "domainId": "#dom", "credentials": pw("admin")}
op(**{"@type": "upsert", "object": "Account", "matchOn": ["name", "domainId"], "value": accounts})

if env("BREVO_LOGIN"):
    op(**{"@type": "upsert", "object": "MtaRoute", "matchOn": ["name"], "value": {"brevo": {
        "@type": "Relay", "name": "brevo", "description": "Brevo SMTP relay (residential IP can't send directly)",
        "address": "smtp-relay.brevo.com", "port": 587, "implicitTls": False, "allowInvalidCerts": False,
        "protocol": "smtp", "authUsername": env("BREVO_LOGIN"),
        "authSecret": {"@type": "EnvironmentVariable", "variableName": "BREVO_SMTP_KEY"}}}})
    op(**{"@type": "update", "object": "MtaOutboundStrategy", "value": {"route": {
        "match": {"0": {"if": "is_local_domain(rcpt_domain)", "then": "'local'"}}, "else": "'brevo'"}}})

print("\n".join(out))
