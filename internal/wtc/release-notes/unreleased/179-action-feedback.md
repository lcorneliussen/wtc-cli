## Collection action feedback

- **Commit changes** asks the conversation to review and commit pending files
  before the person publishes them. The collapsed card points to available
  actions, including pending commits.
- The expanded card shows the latest action result in full, with a dismiss
  control. Publication refusal is shown as an error even when the gateway
  returns a status and message without an error object. Clicking push gives
  immediate feedback naming the selected repository.
