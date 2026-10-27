## Services

- **Changed runtime tasks reach the live runner.** Configuration updates use dekit's native reload before restarting changed running tasks. Newly added tasks become available, removed tasks are stopped, and changes made while down replace the saved definitions on the next start. An already-stale runner can import its generated configuration with `wtc svc dekit -- reload --yes` ([#208](https://github.com/lcorneliussen/wtc-cli/issues/208)).
