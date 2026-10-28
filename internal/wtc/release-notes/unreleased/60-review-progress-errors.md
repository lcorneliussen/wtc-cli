## Fixes

- Independent review concerns and the lead now run locally when progress posting fails. The command preserves completed results and reports posting and execution errors at the end, without retrying an ambiguous write. A local create-intent marker prevents duplicate creates across uncertain retries. Failed updates retain the saved comment ID instead of creating duplicate comments. Full Bitbucket transport and writable-contract verification remain open under #60.
