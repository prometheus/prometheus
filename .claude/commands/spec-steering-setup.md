# Spec Steering Setup Command

Create or update steering documents that provide persistent project context.

## Usage
```
/spec-steering-setup
```

## Instructions
You are helping set up steering documents that will guide all future spec development. These documents provide persistent context about the product vision, technology stack, and project structure.

## Process

1. **Check for Existing Steering Documents**
   - Look for `.claude/steering/` directory
   - Check for existing product.md, tech.md, structure.md files
   - If they exist, load and display current content

2. **Analyze the Project**
   - Review the codebase to understand:
     - Project type and purpose
     - Technology stack in use
     - Directory structure and patterns
     - Coding conventions
     - Existing features and functionality
   - Look for:
     - package.json, requirements.txt, go.mod, etc.
     - README files
     - Configuration files
     - Source code structure

3. **Present Inferred Details**
   - Show the user what you've learned about:
     - **Product**: Purpose, features, target users
     - **Technology**: Frameworks, libraries, tools
     - **Structure**: File organization, naming conventions
   - Format as:
     ```
     Based on my analysis, here's what I've inferred:
     
     **Product Details:**
     - [Inferred detail 1]
     - [Inferred detail 2]
     
     **Technology Stack:**
     - [Inferred tech 1]
     - [Inferred tech 2]
     
     **Project Structure:**
     - [Inferred pattern 1]
     - [Inferred pattern 2]
     ```
   - Ask: "Do these inferred details look correct? Please let me know which ones to keep or discard."

4. **Gather Missing Information**
   - Based on user feedback, identify gaps
   - Ask targeted questions to fill in missing details:
     
     **Product Questions:**
     - What is the main problem this product solves?
     - Who are the primary users?
     - What are the key business objectives?
     - What metrics define success?
     
     **Technology Questions:**
     - Are there any technical constraints or requirements?
     - What third-party services are integrated?
     - What are the performance requirements?
     
     **Structure Questions:**
     - Are there specific coding standards to follow?
     - How should new features be organized?
     - What are the testing requirements?

5. **Generate Steering Documents**
   - Create `.claude/steering/` directory if it doesn't exist
   - Generate three files based on templates and gathered information:
     
     **product.md**: Product vision, users, features, objectives
     **tech.md**: Technology stack, tools, constraints, decisions
     **structure.md**: File organization, naming conventions, patterns

6. **Review and Confirm**
   - Present the generated documents to the user
   - Ask for final approval before saving
   - Make any requested adjustments

## Important Notes

- **Steering documents are persistent** - they will be referenced in all future spec commands
- **Keep documents focused** - each should cover its specific domain
- **Update regularly** - steering docs should evolve with the project
- **Never include sensitive data** - no passwords, API keys, or credentials

## Example Flow

1. Analyze project and find it's a React/TypeScript app
2. Present inferred details about the e-commerce platform
3. User confirms most details but clarifies target market
4. Ask about performance requirements and third-party services
5. Generate steering documents with all gathered information
6. User reviews and approves the documents
7. Save to `.claude/steering/` directory

## Next Steps
After steering documents are created, they will automatically be referenced during:
- `/spec-create` - Align requirements with product vision
- `/spec-design` - Follow established tech patterns
- `/spec-tasks` - Use correct file organization
- `/spec-execute` - Implement following all conventions
