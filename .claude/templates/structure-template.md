# Project Structure

## Directory Organization

```
[Define your project's directory structure. Examples below - adapt to your project type]

Example for a library/package:
project-root/
├── src/                    # Source code
├── tests/                  # Test files  
├── docs/                   # Documentation
├── examples/               # Usage examples
└── [build/dist/out]        # Build output

Example for an application:
project-root/
├── [src/app/lib]           # Main source code
├── [assets/resources]      # Static resources
├── [config/settings]       # Configuration
├── [scripts/tools]         # Build/utility scripts
└── [tests/spec]            # Test files

Common patterns:
- Group by feature/module
- Group by layer (UI, business logic, data)
- Group by type (models, controllers, views)
- Flat structure for simple projects
```

## Naming Conventions

### Files
- **Components/Modules**: [e.g., `PascalCase`, `snake_case`, `kebab-case`]
- **Services/Handlers**: [e.g., `UserService`, `user_service`, `user-service`]
- **Utilities/Helpers**: [e.g., `dateUtils`, `date_utils`, `date-utils`]
- **Tests**: [e.g., `[filename]_test`, `[filename].test`, `[filename]Test`]

### Code
- **Classes/Types**: [e.g., `PascalCase`, `CamelCase`, `snake_case`]
- **Functions/Methods**: [e.g., `camelCase`, `snake_case`, `PascalCase`]
- **Constants**: [e.g., `UPPER_SNAKE_CASE`, `SCREAMING_CASE`, `PascalCase`]
- **Variables**: [e.g., `camelCase`, `snake_case`, `lowercase`]

## Import Patterns

### Import Order
1. External dependencies
2. Internal modules
3. Relative imports
4. Style imports

### Module/Package Organization
```
[Describe your project's import/include patterns]
Examples:
- Absolute imports from project root
- Relative imports within modules
- Package/namespace organization
- Dependency management approach
```

## Code Structure Patterns

[Define common patterns for organizing code within files. Below are examples - choose what applies to your project]

### Module/Class Organization
```
Example patterns:
1. Imports/includes/dependencies
2. Constants and configuration
3. Type/interface definitions
4. Main implementation
5. Helper/utility functions
6. Exports/public API
```

### Function/Method Organization
```
Example patterns:
- Input validation first
- Core logic in the middle
- Error handling throughout
- Clear return points
```

### File Organization Principles
```
Choose what works for your project:
- One class/module per file
- Related functionality grouped together
- Public API at the top/bottom
- Implementation details hidden
```

## Code Organization Principles

1. **Single Responsibility**: Each file should have one clear purpose
2. **Modularity**: Code should be organized into reusable modules
3. **Testability**: Structure code to be easily testable
4. **Consistency**: Follow patterns established in the codebase

## Module Boundaries
[Define how different parts of your project interact and maintain separation of concerns]

Examples of boundary patterns:
- **Core vs Plugins**: Core functionality vs extensible plugins
- **Public API vs Internal**: What's exposed vs implementation details  
- **Platform-specific vs Cross-platform**: OS-specific code isolation
- **Stable vs Experimental**: Production code vs experimental features
- **Dependencies direction**: Which modules can depend on which

## Code Size Guidelines
[Define your project's guidelines for file and function sizes]

Suggested guidelines:
- **File size**: [Define maximum lines per file]
- **Function/Method size**: [Define maximum lines per function]
- **Class/Module complexity**: [Define complexity limits]
- **Nesting depth**: [Maximum nesting levels]

## Dashboard/Monitoring Structure (if applicable)
[How dashboard or monitoring components are organized]

### Example Structure:
```
src/
└── dashboard/          # Self-contained dashboard subsystem
    ├── server/        # Backend server components
    ├── client/        # Frontend assets
    ├── shared/        # Shared types/utilities
    └── public/        # Static assets
```

### Separation of Concerns
- Dashboard isolated from core business logic
- Own CLI entry point for independent operation
- Minimal dependencies on main application
- Can be disabled without affecting core functionality

## Documentation Standards
- All public APIs must have documentation
- Complex logic should include inline comments
- README files for major modules
- Follow language-specific documentation conventions
