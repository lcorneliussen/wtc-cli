# Remember collection tunnel choices

- Service `down` preserves explicit tunnel on/off intent; `up` and `restart` restore enabled tunnels after provider checks. `wtc svc tunnel --stop` keeps them off, overriding declaration defaults.
- Choices are collection-local and bound to the approved definition. Changed or removed declarations need a fresh choice before reopening; public tunnels still require explicit approval.
- Status reports intended-on tunnels that are not running separately from login, reachability and exposure diagnostics.
