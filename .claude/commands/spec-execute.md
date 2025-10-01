# Spec Execute Command

Execute specific tasks from the approved task list.

## Usage
```
/spec-execute [task-id] [feature-name]
```

## Phase Overview
**Your Role**: Execute tasks systematically with validation

This is Phase 4 of the spec workflow. Your goal is to implement individual tasks from the approved task list, one at a time.

## Instructions

**Execution Steps**:

**Step 1: Load Context**
```bash
# Load steering documents (if available)
claude-code-spec-workflow get-steering-context

# Load specification context
claude-code-spec-workflow get-spec-context {feature-name}

# Load specific task details
claude-code-spec-workflow get-tasks {feature-name} {task-id} --mode single
```

**Step 2: Execute with Agent**
Use the `spec-task-executor` agent:
```
Use the spec-task-executor agent to implement task {task-id} for the {feature-name} specification.

## Steering Context
[PASTE THE COMPLETE OUTPUT FROM get-steering-context COMMAND HERE]

## Specification Context
[PASTE THE REQUIREMENTS AND DESIGN SECTIONS FROM get-spec-context COMMAND HERE]

## Task Details
[PASTE THE OUTPUT FROM get-tasks SINGLE COMMAND HERE]

## Instructions
- Implement ONLY the specified task: {task-id}
- Follow all project conventions and leverage existing code
- Mark the task as complete using: claude-code-spec-workflow get-tasks {feature-name} {task-id} --mode complete
- Provide a completion summary
```


3. **Task Execution**
   - Focus on ONE task at a time
   - If task has sub-tasks, start with those
   - Follow the implementation details from design.md
   - Verify against requirements specified in the task

4. **Implementation Guidelines**
   - Write clean, maintainable code
   - **Follow steering documents**: Adhere to patterns in tech.md and conventions in structure.md
   - Follow existing code patterns and conventions
   - Include appropriate error handling
   - Add unit tests where specified
   - Document complex logic

5. **Validation**
   - Verify implementation meets acceptance criteria
   - Run tests if they exist
   - Check for lint/type errors
   - Ensure integration with existing code

6. **Task Completion Protocol**
When completing any task during `/spec-execute`:
   1. **Mark task complete**: Use the get-tasks script to mark completion:
      ```bash
      # Cross-platform command:
      claude-code-spec-workflow get-tasks {feature-name} {task-id} --mode complete
      ```
   2. **Confirm to user**: State clearly "Task X has been marked as complete"
   3. **Stop execution**: Do not proceed to next task automatically
   4. **Wait for instruction**: Let user decide next steps




## Critical Workflow Rules

### Task Execution
- **ONLY** execute one task at a time during implementation
- **CRITICAL**: Mark completed tasks using get-tasks --mode complete before stopping
- **ALWAYS** stop after completing a task
- **NEVER** automatically proceed to the next task
- **MUST** wait for user to request next task execution
- **CONFIRM** task completion status to user

### Requirement References
- **ALL** tasks must reference specific requirements using _Requirements: X.Y_ format
- **ENSURE** traceability from requirements through design to implementation
- **VALIDATE** implementations against referenced requirements

## Task Selection
If no task-id specified:
- Look at tasks.md for the spec
- Recommend the next pending task
- Ask user to confirm before proceeding

If no feature-name specified:
- Check `.claude/specs/` directory for available specs
- If only one spec exists, use it
- If multiple specs exist, ask user which one to use
- Display error if no specs are found

## Examples
```
/spec-execute 1 user-authentication
/spec-execute 2.1 user-authentication
```

## Important Rules
- Only execute ONE task at a time
- **ALWAYS** mark completed tasks using get-tasks --mode complete
- Always stop after completing a task
- Wait for user approval before continuing
- Never skip tasks or jump ahead
- Confirm task completion status to user

## Next Steps
After task completion, you can:
- Address any issues identified in the review
- Run tests if applicable
- Execute the next task using `/spec-execute [next-task-id]`
- Check overall progress with `/spec-status {feature-name}`
