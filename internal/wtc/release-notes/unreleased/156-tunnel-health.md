## Services and tunnels

- **A service or tunnel that stops answering is no longer shown as ready.**
  Ready only meant that a startup probe passed. `wtc svc`, `wtc status` and
  the OpenClaw card now also look at each ready task's address as they
  report. One that gives no answer, or a 5xx such as a tunnel's 502 while the
  service behind it restarts, shows `not answering now` with what came back
  ([#156](https://github.com/lcorneliussen/wtc-cli/issues/156)).
- **A tunnel provider that isn't signed in is named.** A Tailscale tunnel
  whose client needs a login, is stopped or has an expired key, and a
  devtunnel whose login expired, show how to sign in again, and `wtc svc
  tunnel` refuses to open one until then
  ([#156](https://github.com/lcorneliussen/wtc-cli/issues/156)).
