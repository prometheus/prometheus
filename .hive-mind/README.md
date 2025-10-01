# Hive Mind System

This directory contains the Claude Flow Hive Mind system configuration and data.

## Directory Structure

- **config/**: Configuration files for queens, workers, and system settings
- **sessions/**: Active and historical session data
- **memory/**: Collective memory and knowledge base
- **logs/**: System and debug logs
- **backups/**: Automated backups of system state
- **templates/**: Templates for agents and workflows
- **exports/**: Exported data and reports

## Database Files

- **hive.db**: Main SQLite database (or memory.json as fallback)
- **config.json**: Primary system configuration

## Getting Started

1. Initialize: `npx claude-flow@alpha hive-mind init`
2. Spawn swarm: `npx claude-flow@alpha hive-mind spawn "your objective"`
3. Check status: `npx claude-flow@alpha hive-mind status`

## Features

- **Collective Intelligence**: Multiple AI agents working together
- **Consensus Building**: Democratic decision-making process
- **Adaptive Learning**: System improves over time
- **Fault Tolerance**: Self-healing and recovery capabilities
- **Performance Monitoring**: Real-time metrics and optimization

## Configuration

Edit `.hive-mind/config.json` to customize:
- Queen type and capabilities
- Worker specializations
- Consensus algorithms
- Memory settings
- Integration options

For more information, see the [Hive Mind Documentation](https://github.com/ruvnet/claude-flow/docs/hive-mind.md).
