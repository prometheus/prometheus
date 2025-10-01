# Spec Status Command

Show current status of all specs or a specific spec.

## Usage
```
/spec-status [feature-name]
```

## Instructions
Display the current status of spec workflows.

1. **If no feature-name provided:**
   - List all specs in `.claude/specs/` directory
   - Show current phase for each spec
   - Display completion status

2. **If feature-name provided:**
   - Show detailed status for that spec
   - Display current workflow phase
   - Show completed vs pending tasks
   - List next recommended actions

3. **Status Information:**
   - Requirements: [Complete/In Progress/Pending]
   - Design: [Complete/In Progress/Pending]
   - Tasks: [Complete/In Progress/Pending]
   - Implementation: [X/Y tasks complete]

4. **Output Format:**
   ```
   Spec: user-authentication
   Phase: Implementation
   Progress: Requirements ✅ | Design ✅ | Tasks ✅
   Implementation: 3/8 tasks complete
   Next: Execute task 4 - "Implement password validation"
   ```

## Workflow Phases
- **Requirements**: Gathering and documenting requirements
- **Design**: Creating technical design and architecture
- **Tasks**: Breaking down into implementation tasks
- **Implementation**: Executing individual tasks
- **Complete**: All tasks finished and integrated
