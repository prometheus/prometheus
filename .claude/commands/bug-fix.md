# Bug Fix Command

Implement the fix for the analyzed bug.

## Usage
```
/bug-fix [bug-name]
```

## Phase Overview
**Your Role**: Implement the solution based on the approved analysis

This is Phase 3 of the bug fix workflow. Your goal is to implement the fix while following project conventions.

## Instructions

You are working on the fix implementation phase of the bug fix workflow.

1. **Prerequisites & Context Loading**
   - Ensure analysis.md exists and is approved

   **Load ALL Context Once (Hierarchical Context Loading):**
   ```bash
   # Load steering documents (if available)
   claude-code-spec-workflow get-steering-context
   ```

   **Bug documents to read directly:**
   - `.claude/bugs/{bug-name}/report.md`
   - `.claude/bugs/{bug-name}/analysis.md`
   - Understand the planned fix approach completely

2. **Implementation Process**
   1. **Follow the Implementation Plan**
      - Execute changes exactly as outlined in analysis.md
      - Make targeted, minimal changes
      - Follow existing code patterns and conventions

   2. **Code Changes**
      - Implement the fix following project standards
      - Add appropriate error handling
      - Include logging or debugging aids if needed
      - Update or add tests as specified

   3. **Quality Checks**
      - Verify fix addresses the root cause
      - Ensure no unintended side effects
      - Follow code style and conventions
      - Run tests and checks

3. **Implementation Guidelines**
   - **Follow steering documents**: Adhere to patterns in tech.md and conventions in structure.md
   - **Make minimal changes**: Fix only what's necessary
   - **Preserve existing behavior**: Don't break unrelated functionality
   - **Use existing patterns**: Leverage established code patterns and utilities
   - **Add appropriate tests**: Ensure the bug won't return

4. **Testing Requirements**
   - Test the specific bug scenario
   - Verify related functionality still works
   - Run existing test suite if available
   - Add regression tests for this bug

5. **Documentation Updates**
   - Update code comments if needed
   - Document any non-obvious changes
   - Update error messages if applicable

## Implementation Rules

### Code Quality
- Follow project coding standards
- Use existing utilities and patterns
- Add proper error handling
- Include meaningful comments for complex logic

### Testing Strategy
- Test the original bug reproduction steps
- Verify fix doesn't break related functionality
- Add tests to prevent regression
- Run full test suite if available

### Change Management
- Make atomic, focused changes
- Document the fix approach
- Preserve existing API contracts
- Consider backwards compatibility

## Completion Process

1. **Implement the Fix**
   - Make the necessary code changes
   - Follow the implementation plan from analysis.md
   - Ensure code follows project conventions

2. **Verify Implementation**
   - Test that the original bug is resolved
   - Verify no new issues introduced
   - Run relevant tests and checks

3. **Update Documentation**
   - Document the changes made
   - Update any relevant comments or docs

4. **Confirm Completion**
   - Present summary of changes made
   - Show test results confirming fix

5. **Final Confirmation**
   - Ask: "The fix has been implemented and reviewed. Should we proceed to verification?"
   - **CRITICAL**: Wait for user approval before proceeding

## Critical Rules
- **ONLY** implement the fix outlined in the approved analysis
- **ALWAYS** test the fix thoroughly
- **NEVER** make changes beyond the planned fix scope
- **MUST** wait for user approval before proceeding to verification

## Next Phase
After approval, proceed to `/bug-verify`.
