# kube-freeze-operator Documentation

Welcome to kube-freeze-operator documentation!

## Table of Contents

- [Architecture](architecture.md) - System design and components
- [Usage Guide](usage.md) - How to use freeze policies
- [API Reference](api-reference.md) - CRD specifications
- [Troubleshooting](troubleshooting.md) - Common issues and solutions
- [Security](security.md) - Security best practices

## Quick Links

- [Installation Guide](../README.md#getting-started)
- [Examples](../examples/)
- [Contributing](../CONTRIBUTING.md)

## Overview

kube-freeze-operator is a Kubernetes operator that enforces change freeze and maintenance windows for workloads. It provides:

- **MaintenanceWindow**: Define allowed time windows for changes
- **ChangeFreeze**: Block changes during specific periods
- **FreezeException**: Override freeze policies for emergency changes

## Getting Help

- 📖 Read the documentation
- 🐛 Report bugs via GitHub Issues
- 💬 Ask questions in discussions
- 🤝 Contribute via pull requests
