# Spec List Command

List all specs in the current project.

## Usage
```
/spec-list
```

## Instructions
Display a comprehensive list of all specs in the project.

1. **Scan Directory**
   - Look in `.claude/specs/` directory
   - Find all spec directories
   - Check for required files (requirements.md, design.md, tasks.md)

2. **Display Information**
   - Feature name
   - Current phase
   - Completion status
   - Last modified date
   - Brief description from requirements

3. **Output Format**
   ```
   📋 Project Specs Overview

   1. user-authentication (Complete)
      Phase: Implementation (7/8 tasks)
      Last updated: 2025-01-15

   2. data-export (In Progress)
      Phase: Design
      Last updated: 2025-01-14

   3. notification-system (Planning)
      Phase: Requirements
      Last updated: 2025-01-13
   ```

4. **Additional Actions**
   - Show total spec count
   - Highlight specs needing attention
   - Suggest next actions for each spec
