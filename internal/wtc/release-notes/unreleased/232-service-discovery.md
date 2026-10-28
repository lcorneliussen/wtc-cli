## Services

- Service lists discover shared and collection-local task declarations before
  their first start, including tasks in a newly added repository. New tasks show
  as `unconfigured`; existing runtime and tunnel observations keep precedence.
  Discovery reads definitions without executing hooks, sourcing environment files
  or writing generated runtime state.
