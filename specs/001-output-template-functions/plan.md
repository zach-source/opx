
# Implementation Plan: Output Template Functions

**Branch**: `001-output-template-functions` | **Date**: 2025-09-29 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-output-template-functions/spec.md`

## Execution Flow (/plan command scope)
```
1. Load feature spec from Input path
   → If not found: ERROR "No feature spec at {path}"
2. Fill Technical Context (scan for NEEDS CLARIFICATION)
   → Detect Project Type from file system structure or context (web=frontend+backend, mobile=app+api)
   → Set Structure Decision based on project type
3. Fill the Constitution Check section based on the content of the constitution document.
4. Evaluate Constitution Check section below
   → If violations exist: Document in Complexity Tracking
   → If no justification possible: ERROR "Simplify approach first"
   → Update Progress Tracking: Initial Constitution Check
5. Execute Phase 0 → research.md
   → If NEEDS CLARIFICATION remain: ERROR "Resolve unknowns"
6. Execute Phase 1 → contracts, data-model.md, quickstart.md, agent-specific template file (e.g., `CLAUDE.md` for Claude Code, `.github/copilot-instructions.md` for GitHub Copilot, `GEMINI.md` for Gemini CLI, `QWEN.md` for Qwen Code or `AGENTS.md` for opencode).
7. Re-evaluate Constitution Check section
   → If new violations: Refactor design, return to Phase 1
   → Update Progress Tracking: Post-Design Constitution Check
8. Plan Phase 2 → Describe task generation approach (DO NOT create tasks.md)
9. STOP - Ready for /tasks command
```

**IMPORTANT**: The /plan command STOPS at step 7. Phases 2-4 are executed by other commands:
- Phase 2: /tasks command creates tasks.md
- Phase 3-4: Implementation execution (manual or via tools)

## Summary
Add Golang Sprig template functions to opx commands enabling users to transform secret output using functions like base64 encoding, default values, and other safe string/math operations. Templates are embedded in reference paths using query parameter syntax or provided via command-line flags.

## Technical Context
**Language/Version**: Go 1.24+ (using generics, SafeString)
**Primary Dependencies**: Masterminds/sprig v3, text/template (stdlib), go.uber.org/zap (existing)
**Storage**: N/A (stateless template processing)
**Testing**: go test with fake backends (existing pattern)
**Target Platform**: Unix-like systems (Linux, macOS) via opx daemon
**Project Type**: Single binary CLI + daemon architecture
**Performance Goals**: <5s template execution, <1ms template compilation cache
**Constraints**: Sandboxed execution, no OS/network/file access, 5s timeout
**Scale/Scope**: Individual secret processing, batch operations up to 100 refs

## Constitution Check
*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**I. Security-First Architecture**: ✅ PASS
- Template execution will be sandboxed with restricted function set
- No OS/network/file access in allowed Sprig functions
- Input validation prevents template injection

**II. Zero-Trust Architecture**: ✅ PASS
- All template input will be validated before processing
- Reference format validation enforced (query parameter syntax)
- Template syntax validation prevents code injection

**III. Defense in Depth**: ✅ PASS
- Template processing adds new validation layer
- Existing security layers (TLS, auth, policy) remain intact
- Timeout protection prevents DoS via template execution

**V. Testing Discipline**: ✅ PASS
- Will add comprehensive tests for template parsing and execution
- Fake backends enable testing without external dependencies
- Integration tests for end-to-end template workflows

**VI. Backend Abstraction**: ✅ PASS
- Template processing occurs after backend secret retrieval
- No backend-specific template logic
- Preserves clean abstraction boundaries

**IX. Structured Logging**: ✅ PASS
- Template processing errors will use existing Zap logger
- Debug logging for template compilation and execution
- No secrets in log output (template results redacted)

## Progress Tracking

### Initial Constitution Check: ✅ COMPLETE
- All security and architecture principles verified
- Template processing aligns with security-first approach
- Function allowlisting prevents security violations

### Phase 0 (Research): ✅ COMPLETE
- Technical decisions documented in research.md
- Sprig integration strategy defined
- Security model with function allowlisting established

### Phase 1 (Design): ✅ COMPLETE
- Data model created with core entities
- API contracts defined for template processing
- Quickstart scenarios for testing
- Claude Code implementation guidelines created

### Post-Design Constitution Check: ✅ PASS
- Template processing preserves backend abstraction
- Security boundaries maintained with function restrictions
- Logging integration follows structured logging principle

## Phase 2 Planning: Task Generation Approach

### Task Categories
1. **Setup Tasks**: Dependencies, package creation, basic structure
2. **Core Implementation**: Template processor, function registry, caching
3. **Integration Tasks**: Reference parsing, command updates, server integration
4. **Testing Tasks**: Unit tests, security tests, integration tests
5. **Polish Tasks**: Performance optimization, documentation

### Parallel Execution Strategy
- **[P] Template package creation** - independent files can be developed in parallel
- **[P] Unit tests** - can be written alongside implementation
- **Sequential integration** - server changes must be done carefully to avoid conflicts

### Dependency Order
1. Setup → Core Implementation
2. Core Implementation → Integration
3. Integration → Testing
4. Everything → Polish

Ready for `/tasks` command to generate specific implementation tasks.

## Project Structure

### Documentation (this feature)
```
specs/[###-feature]/
├── plan.md              # This file (/plan command output)
├── research.md          # Phase 0 output (/plan command)
├── data-model.md        # Phase 1 output (/plan command)
├── quickstart.md        # Phase 1 output (/plan command)
├── contracts/           # Phase 1 output (/plan command)
└── tasks.md             # Phase 2 output (/tasks command - NOT created by /plan)
```

### Source Code (repository root)
<!--
  ACTION REQUIRED: Replace the placeholder tree below with the concrete layout
  for this feature. Delete unused options and expand the chosen structure with
  real paths (e.g., apps/admin, packages/something). The delivered plan must
  not include Option labels.
-->
```
# [REMOVE IF UNUSED] Option 1: Single project (DEFAULT)
src/
├── models/
├── services/
├── cli/
└── lib/

tests/
├── contract/
├── integration/
└── unit/

# [REMOVE IF UNUSED] Option 2: Web application (when "frontend" + "backend" detected)
backend/
├── src/
│   ├── models/
│   ├── services/
│   └── api/
└── tests/

frontend/
├── src/
│   ├── components/
│   ├── pages/
│   └── services/
└── tests/

# [REMOVE IF UNUSED] Option 3: Mobile + API (when "iOS/Android" detected)
api/
└── [same as backend above]

ios/ or android/
└── [platform-specific structure: feature modules, UI flows, platform tests]
```

**Structure Decision**: [Document the selected structure and reference the real
directories captured above]

## Phase 0: Outline & Research
1. **Extract unknowns from Technical Context** above:
   - For each NEEDS CLARIFICATION → research task
   - For each dependency → best practices task
   - For each integration → patterns task

2. **Generate and dispatch research agents**:
   ```
   For each unknown in Technical Context:
     Task: "Research {unknown} for {feature context}"
   For each technology choice:
     Task: "Find best practices for {tech} in {domain}"
   ```

3. **Consolidate findings** in `research.md` using format:
   - Decision: [what was chosen]
   - Rationale: [why chosen]
   - Alternatives considered: [what else evaluated]

**Output**: research.md with all NEEDS CLARIFICATION resolved

## Phase 1: Design & Contracts
*Prerequisites: research.md complete*

1. **Extract entities from feature spec** → `data-model.md`:
   - Entity name, fields, relationships
   - Validation rules from requirements
   - State transitions if applicable

2. **Generate API contracts** from functional requirements:
   - For each user action → endpoint
   - Use standard REST/GraphQL patterns
   - Output OpenAPI/GraphQL schema to `/contracts/`

3. **Generate contract tests** from contracts:
   - One test file per endpoint
   - Assert request/response schemas
   - Tests must fail (no implementation yet)

4. **Extract test scenarios** from user stories:
   - Each story → integration test scenario
   - Quickstart test = story validation steps

5. **Update agent file incrementally** (O(1) operation):
   - Run `.specify/scripts/bash/update-agent-context.sh claude`
     **IMPORTANT**: Execute it exactly as specified above. Do not add or remove any arguments.
   - If exists: Add only NEW tech from current plan
   - Preserve manual additions between markers
   - Update recent changes (keep last 3)
   - Keep under 150 lines for token efficiency
   - Output to repository root

**Output**: data-model.md, /contracts/*, failing tests, quickstart.md, agent-specific file

## Phase 2: Task Planning Approach
*This section describes what the /tasks command will do - DO NOT execute during /plan*

**Task Generation Strategy**:
- Load `.specify/templates/tasks-template.md` as base
- Generate tasks from Phase 1 design docs (contracts, data model, quickstart)
- Each contract → contract test task [P]
- Each entity → model creation task [P] 
- Each user story → integration test task
- Implementation tasks to make tests pass

**Ordering Strategy**:
- TDD order: Tests before implementation 
- Dependency order: Models before services before UI
- Mark [P] for parallel execution (independent files)

**Estimated Output**: 25-30 numbered, ordered tasks in tasks.md

**IMPORTANT**: This phase is executed by the /tasks command, NOT by /plan

## Phase 3+: Future Implementation
*These phases are beyond the scope of the /plan command*

**Phase 3**: Task execution (/tasks command creates tasks.md)  
**Phase 4**: Implementation (execute tasks.md following constitutional principles)  
**Phase 5**: Validation (run tests, execute quickstart.md, performance validation)

## Complexity Tracking
*Fill ONLY if Constitution Check has violations that must be justified*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |


## Progress Tracking
*This checklist is updated during execution flow*

**Phase Status**:
- [ ] Phase 0: Research complete (/plan command)
- [ ] Phase 1: Design complete (/plan command)
- [ ] Phase 2: Task planning complete (/plan command - describe approach only)
- [ ] Phase 3: Tasks generated (/tasks command)
- [ ] Phase 4: Implementation complete
- [ ] Phase 5: Validation passed

**Gate Status**:
- [ ] Initial Constitution Check: PASS
- [ ] Post-Design Constitution Check: PASS
- [ ] All NEEDS CLARIFICATION resolved
- [ ] Complexity deviations documented

---
*Based on Constitution v2.1.1 - See `/memory/constitution.md`*
