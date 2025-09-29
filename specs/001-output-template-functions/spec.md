# Feature Specification: Output Template Functions with Sprig

**Feature Branch**: `001-output-template-functions`
**Created**: 2025-09-28
**Status**: Draft
**Input**: User description: "output template functions use golang sprig functions to provide base64 and other helpful functions like default to the opx commands"

## Execution Flow (main)
```
1. Parse user description from Input
   → Feature adds Golang Sprig template functions to opx output formatting
2. Extract key concepts from description
   → Actors: opx users, system administrators
   → Actions: format secret output, apply transformations
   → Data: secret values from backends
   → Constraints: template security, injection prevention
3. For each unclear aspect:
   → Template syntax specification needed
   → Security validation requirements needed
4. Fill User Scenarios & Testing section
   → Primary flow: user requests templated output
5. Generate Functional Requirements
   → Template processing, security validation, error handling
6. Identify Key Entities
   → Template definitions, output formats
7. Run Review Checklist
   → Spec ready for clarification if needed
8. Return: SUCCESS (spec ready for planning)
```

## Clarifications

### Session 2025-09-28
- Q: Which security approach should the template system use for Sprig functions? → A: Safe subset only (strings, encoding, math - no OS/network/file functions)
- Q: Which template input method should be supported? → A: Command-line flag only, and in the reference path like op://foo/bar#secret{{ .Value | default "foo" | base64 }}
- Q: How should templating interact with the resolve command? → A: Yes, apply templates to resolved environment variable values
- Q: What timeout value is appropriate for template execution? → A: all templates should finish in under 5 seconds
- Q: What syntax should separate field name from template in reference paths? → A: Template query parameter (op://vault/item/field?template={{...}})

---

## ⚡ Quick Guidelines
- ✅ Focus on WHAT users need and WHY
- ❌ Avoid HOW to implement (no tech stack, APIs, code structure)
- 👥 Written for business stakeholders, not developers

## User Scenarios & Testing

### Primary User Story
As a system administrator or developer, I want to format secret output using template functions so that I can transform secrets into the required format for my applications without exposing raw values or requiring manual string manipulation.

### Acceptance Scenarios
1. **Given** a secret reference with embedded template, **When** user runs `opx read "op://vault/item/field?template={{.Value | base64}}"`, **Then** the output is base64-encoded
2. **Given** a secret with potential null value, **When** user runs `opx read "op://vault/item/optional?template={{.Value | default \"fallback\"}}"`, **Then** the output shows the fallback value if secret is empty
3. **Given** multiple secrets with templates, **When** user runs batch read, **Then** all outputs are consistently formatted according to their respective templates
4. **Given** an invalid template syntax, **When** user provides malformed template in reference path, **Then** system returns clear error message
5. **Given** environment variable resolution with template, **When** user runs `opx resolve DB_URL="op://vault/db/url?template={{.Value | base64}}"`, **Then** resolved value is template-processed

### Edge Cases
- What happens when template processing fails due to data type mismatch?
- How does system handle templates that would expose sensitive data inappropriately?
- What happens when template execution times out or consumes excessive resources?
- How are nested template references handled (templates calling other templates)?

## Requirements

### Functional Requirements
- **FR-001**: System MUST support template syntax for formatting secret output values
- **FR-002**: System MUST provide base64 encoding/decoding template functions
- **FR-003**: System MUST provide default value template functions for handling empty/null secrets
- **FR-004**: System MUST validate template syntax before execution to prevent injection attacks
- **FR-005**: System MUST support templating for single secret reads (`opx read`)
- **FR-006**: System MUST support templating for batch secret reads (`opx reads`)
- **FR-007**: System MUST provide error messages when template processing fails
- **FR-008**: System MUST support safe subset of Sprig functions including strings, encoding, and math functions while excluding OS, network, and file system functions
- **FR-009**: System MUST support templates via command-line flag and embedded in reference paths using query parameter syntax (e.g., `op://vault/item/field?template={{.Value | base64}}`)
- **FR-010**: System MUST apply template functions to resolved environment variable values in resolve command

### Security Requirements
- **SR-001**: Template execution MUST be sandboxed to prevent code injection
- **SR-002**: Template functions MUST NOT allow file system access
- **SR-003**: Template functions MUST NOT allow network access
- **SR-004**: Template processing MUST have execution timeout to prevent DoS
- **SR-005**: Dangerous template functions (OS, network, file system access) MUST be disabled and excluded from available function set

### Performance Requirements
- **PR-001**: Template processing MUST NOT significantly impact secret retrieval performance
- **PR-002**: Template compilation MUST be cached when possible
- **PR-003**: Template execution MUST have 5-second timeout to prevent DoS and ensure responsive operation

### Key Entities
- **Template Definition**: User-provided string containing template syntax with Sprig functions
- **Secret Value**: Raw value from backend that serves as input to template processing
- **Formatted Output**: Final processed result after template function application
- **Template Context**: Data structure passed to template containing secret value and metadata

---

## Review & Acceptance Checklist

### Content Quality
- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

### Requirement Completeness
- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

---

## Execution Status

- [x] User description parsed
- [x] Key concepts extracted
- [x] Ambiguities marked
- [x] User scenarios defined
- [x] Requirements generated
- [x] Entities identified
- [x] Review checklist passed

---