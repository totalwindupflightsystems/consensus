# Workflow Orchestrator Reference

## Overview
Workflow Orchestrator (inspired by Ralph Wiggum loops) enables AI-driven development workflows that iterate until tasks complete successfully. This reference provides comprehensive guidance on designing, implementing, and optimizing AI workflow patterns.

## Core Concepts

### What are Ralph Loops?
Ralph loops are AI-driven development workflows named after the *Ralph Wiggum* character from The Simpsons (known for persistent, naive optimism). The technique involves:

- **Feeding failures back into AI input** - Using unsuccessful attempts as learning material for subsequent tries
- **Creating contextual pressure cookers** - Concentrated iteration cycles focused on specific problems
- **Using stop hooks** - Detecting completion promises (`<promise>...</promise>` tags) to know when to stop
- **Iterating until success** - Continuing attempts until success criteria are met or limits reached

### Key Principles
1. **Clear Success Criteria**: Workflows need unambiguous completion signals
2. **Intelligent Failure Feedback**: Failed attempts should inform subsequent tries
3. **Controlled Iteration**: Limits prevent infinite loops while allowing sufficient attempts
4. **Multi-Perspective Approaches**: Different agents bring different strengths
5. **Progressive Refinement**: Each iteration should build on previous attempts

## Workflow Patterns

### 1. Builder Only Pattern
**Simple single-agent loop** that retries until success.

**Use when**: Simple tasks, bug fixes, small features where quality is secondary to completion
```yaml
workflow:
  agents:
    builder:
      role: "Build the feature"
      stop_condition: "<promise>BUILD_COMPLETE</promise>"
  sequence:
    - agent: builder
```

**Pros**: Fast, inexpensive, simple  
**Cons**: No quality verification, can produce poor results

### 2. Build + Verify Pattern
**Two-agent sequential workflow** with quality verification.

**Use when**: Production code, quality-critical work, security-sensitive tasks
```yaml
workflow:
  agents:
    builder:
      role: "Build the feature"
      stop_condition: "<promise>BUILT</promise>"
    verifier:
      role: "Verify quality"
      stop_condition: "<promise>VERIFIED</promise>"
  sequence:
    - agent: builder
    - agent: verifier
      condition: "builder.stop_condition met"
  feedback:
    verifier_failed: "builder"
```

**Pros**: Quality assurance, catches errors early  
**Cons**: Slower, more expensive

### 3. Build + Verify + Plan Pattern
**Three-agent workflow** with planning, building, and verification.

**Use when**: Complex features, architectural changes, major refactoring
```yaml
workflow:
  agents:
    planner:
      role: "Plan approach"
      stop_condition: "<promise>PLANNED</promise>"
    builder:
      role: "Build implementation"
      stop_condition: "<promise>BUILT</promise>"
    verifier:
      role: "Verify quality"
      stop_condition: "<promise>VERIFIED</promise>"
  sequence:
    - agent: planner
    - agent: builder
      condition: "planner.stop_condition met"
    - agent: verifier
      condition: "builder.stop_condition met"
  feedback:
    verifier_failed: "planner"
    builder_stuck: "planner"
```

**Pros**: Comprehensive, high-quality results, handles complexity well  
**Cons**: Slow, expensive, complex to configure

### 4. Multi-Agent Pipeline Pattern
**Parallel agent execution** with result merging.

**Use when**: Multiple aspects that can be worked on simultaneously, comprehensive solutions
```yaml
workflow:
  agents:
    specialist_a:
      role: "Handle aspect A"
      stop_condition: "<promise>ASPECT_A_COMPLETE</promise>"
    specialist_b:
      role: "Handle aspect B"
      stop_condition: "<promise>ASPECT_B_COMPLETE</promise>"
    merger:
      role: "Merge parallel work"
      stop_condition: "<promise>MERGED</promise>"
  parallel:
    - specialist_a
    - specialist_b
  sequence:
    - parallel: [specialist_a, specialist_b]
    - agent: merger
      condition: "specialist_a.stop_condition met AND specialist_b.stop_condition met"
```

**Pros**: Fast for parallelizable work, multiple expert perspectives  
**Cons**: Expensive, merging can be challenging

### 5. Custom Pattern
**User-defined sequence** of agents with custom routing logic.

**Use when**: Unique requirements, specialized workflows not covered by standard patterns

## Agent Configuration

### Agent Properties
```yaml
agent_name:
  # Required properties
  role: "Clear description of agent's responsibility"
  model: "anthropic/claude-sonnet-4-20250514"  # Model to use
  temperature: 0.7  # Creativity vs consistency (0.0-1.0)
  
  # Tool configuration
  tools:
    read: true      # Read files
    write: true     # Write new files
    edit: true      # Edit existing files
    bash: true      # Execute bash commands
    webfetch: true  # Fetch web content
    question: true  # Ask user questions
    
  # Stop condition
  stop_condition: "<promise>TASK_COMPLETE</promise>"
  
  # Permissions (optional)
  permissions:
    file_edits: "allow"    # allow, ask, deny
    bash_commands: "allow"  # allow, ask, deny
    git_commands: "ask"     # allow, ask, deny
    
  # Resource limits (optional)
  limits:
    max_cost: 10.00     # Maximum cost in dollars
    max_time_minutes: 30  # Maximum time in minutes
    max_iterations: 10   # Maximum iterations for this agent
```

### Stop Conditions
Stop conditions use promise tags to signal completion:
```xml
<promise>TASK_COMPLETE</promise>
<promise>BUILD_COMPLETE</promise>
<promise>VERIFIED</promise>
<promise>PLANNED</promise>
```

**Best practices**:
- Use specific, unambiguous promise tags
- Document what each promise means
- Train agents to output promises consistently
- Use regex patterns to detect promises in output

## Workflow Configuration

### Global Settings
```yaml
workflow:
  name: "workflow-name"
  version: "1.0"
  
  settings:
    max_iterations: 20           # Global iteration limit
    stop_on_success: true        # Stop when any success criteria met
    escape_hatch: true           # Allow manual interruption
    cost_limit: 50.00            # Maximum total cost
    time_limit_minutes: 120      # Maximum total time
    
  # Agent definitions
  agents:
    agent1: ...
    agent2: ...
    
  # Execution order
  sequence:
    - agent: agent1
    - agent: agent2
      condition: "agent1.stop_condition met"
      
  # Parallel execution
  parallel:
    - agent1
    - agent2
    
  # Hybrid execution
  sequence:
    - parallel: [agent1, agent2]
    - agent: agent3
      condition: "agent1.stop_condition met AND agent2.stop_condition met"
      
  # Feedback loops
  feedback:
    agent2_failed:
      target: agent1
      message: "Verification failed, fix issues"
      
    agent3_stuck:
      target: agent1
      message: "Stuck, needs guidance"
      
  # Success criteria
  success_criteria:
    - "All tests pass"
    - "No linting errors"
    - "Documentation complete"
    - "Performance benchmarks met"
    
  # Telemetry
  telemetry:
    track_costs: true
    track_iterations: true
    track_duration: true
    dashboard_url: "http://localhost:3000/dashboard"
```

## Execution Flow

### Workflow Lifecycle
```
1. Initialization
   ↓
2. Agent Selection & Configuration
   ↓
3. Iteration Loop Start
   ↓
4. Agent Execution
   ↓
5. Stop Condition Check
   ↓
6. Success Criteria Evaluation
   ↓
7. Feedback & Next Iteration
   ↓
8. Completion or Limit Reached
```

### Iteration Management
```python
class WorkflowExecutor:
    def execute_workflow(self, config):
        iteration = 0
        success = False
        
        while iteration < config.max_iterations and not success:
            iteration += 1
            
            # Execute current agent(s)
            results = self.execute_agents(config)
            
            # Check stop conditions
            if self.check_stop_conditions(results):
                # Move to next agent in sequence
                config.advance_to_next_agent()
            
            # Check success criteria
            success = self.evaluate_success_criteria(results)
            
            # Apply feedback if not successful
            if not success:
                self.apply_feedback(config, results)
        
        return {
            'success': success,
            'iterations': iteration,
            'results': results,
            'cost': self.calculate_cost(results)
        }
```

## Integration Patterns

### OpenCode Server Integration
Use OpenCode's HTTP server for workflow execution:
```bash
# Start OpenCode server
opencode serve --port 3000

# Access API documentation
curl "http://localhost:3000/doc?format=json"
```

**API endpoints**:
- `POST /execute` - Execute workflow
- `GET /status/:id` - Check workflow status
- `GET /results/:id` - Get workflow results
- `DELETE /cancel/:id` - Cancel running workflow

### SSE Event Streaming
Real-time monitoring with Server-Sent Events:
```javascript
// Client-side monitoring
const eventSource = new EventSource('/workflow/events');

eventSource.onmessage = (event) => {
    const data = JSON.parse(event.data);
    updateDashboard(data);
};

eventSource.addEventListener('agent_start', ...);
eventSource.addEventListener('agent_complete', ...);
eventSource.addEventListener('iteration_complete', ...);
eventSource.addEventListener('workflow_complete', ...);
```

### TUI Dashboards
Terminal User Interface for workflow monitoring:
```python
import textual
from textual.app import App

class WorkflowDashboard(App):
    def compose(self):
        yield Header()
        yield AgentStatusPanel()
        yield IterationProgress()
        yield CostTracker()
        yield LogViewer()
```

## Best Practices

### 1. Start Simple
- Begin with Builder Only pattern
- Gradually add complexity as needed
- Test workflows with simple tasks first

### 2. Define Clear Success Criteria
- Make success criteria testable and unambiguous
- Use multiple criteria for complex tasks
- Include both functional and quality requirements

### 3. Implement Escape Hatches
- Always set iteration limits
- Allow manual interruption
- Monitor costs and time
- Log everything for debugging

### 4. Optimize Feedback Loops
- Failed verification should provide specific feedback
- Stuck agents should get guidance, not just retry
- Learning from failures improves subsequent attempts

### 5. Monitor and Adjust
- Track costs per iteration
- Measure success rates
- Adjust patterns based on historical performance
- A/B test different workflow configurations

## Common Issues & Solutions

### Issue: Infinite Loops
**Symptoms**: Workflow never completes, hits iteration limits
**Solutions**:
- Clearer success criteria
- Better stop condition detection
- More specific agent instructions
- Shorter iteration limits with progress checks

### Issue: Poor Quality Results
**Symptoms**: Tasks complete but with low quality
**Solutions**:
- Add verification agent
- Implement quality gates
- Define specific quality criteria
- Use specialized agents for quality checks

### Issue: High Costs
**Symptoms**: Workflow completes but costs are excessive
**Solutions**:
- Use cheaper models for simple tasks
- Implement cost tracking and limits
- Optimize agent tool usage
- Parallelize where possible to reduce time

### Issue: Agent Stuck
**Symptoms**: Agent makes no progress across iterations
**Solutions**:
- Add planning agent for guidance
- Implement feedback to different agent type
- Provide more context or examples
- Simplify the task or break it down

## Tools & Utilities

### Scripts Provided
- `list-patterns.sh` - List available workflow patterns
- `analyze-task.sh` - Analyze task and recommend pattern
- `generate-workflow.sh` - Generate workflow configuration

### Pattern Templates
- `builder-only.yaml` - Simple single-agent workflow
- `build-verify-plan.yaml` - Three-agent planned workflow
- `multi-agent-pipeline.yaml` - Parallel agent workflow

### Validation Tools
```bash
# Validate workflow configuration
npm run workflow:validate -- --config workflow.yaml

# Lint workflow configuration
npm run workflow:lint -- --config workflow.yaml

# Test workflow with sample task
npm run workflow:test -- --pattern build-verify --task "simple test"
```

## Further Reading
- [OpenCode Server API Documentation](https://opencode.ai/docs)
- [AI Workflow Patterns Research](https://example.com/ai-workflow-patterns)
- [Multi-Agent Systems Design](https://example.com/multi-agent-design)
- [Cost Optimization for AI Workflows](https://example.com/ai-cost-optimization)