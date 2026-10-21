## OpenClaw: install

- **`wtc openclaw install` works on a gateway that has not consented to the
  plugin yet**
  ([#134](https://github.com/lcorneliussen/wtc-cli/issues/134)). It failed
  with "requires capability consent", for example on a freshly configured
  gateway. It now passes `--accept-capabilities`: running it is the
  operator's consent to the panel and session actions the plugin declares.
  `--dir` prints the same flag for installing on another host.
