## Services

- **Tunnel exposure is named in status.** Public addresses show `[public]`, and their color follows service health. Running tunnels with unknown exposure are labeled explicitly and remain warning-colored; plain service output names exposure too. The runtime-status contract now has end-to-end wiring coverage for live reachability and provider-login observations, while startup polling keeps its runner-only view ([#157](https://github.com/lcorneliussen/wtc-cli/issues/157), [#172](https://github.com/lcorneliussen/wtc-cli/issues/172)).
