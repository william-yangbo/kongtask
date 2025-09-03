# KongTask Documentation

This directory contains comprehensive documentation for KongTask, providing detailed guides and references for all aspects of the job queue system.

## Documentation Structure

### 📋 [API Reference](API_REFERENCE.md)

Complete API documentation covering all interfaces, types, and functions available in KongTask.

**Contents:**

- Core interfaces (TaskHandler, Logger, EventBus)
- Main functions (RunTaskList, RunTaskListOnce, RunMigrations)
- Worker types and configuration options
- Job specifications and management
- Administrative functions
- Cron support
- Event system
- Error handling
- Usage examples

### ⏰ [Crontab Guide](CRONTAB.md)

Comprehensive guide for using cron scheduling functionality in KongTask.

**Contents:**

- Crontab syntax and expressions
- Time zone handling
- Configuration options
- Advanced patterns and examples
- Best practices for scheduled tasks
- Troubleshooting cron jobs

### 🛠️ [Task Handlers](TASK_HANDLERS.md)

Go-specific best practices for creating robust and efficient task handlers.

**Contents:**

- TaskHandler interface implementation
- Error handling patterns
- Context management and timeouts
- Database operations
- Testing strategies
- Performance optimization
- Resource cleanup
- Helper utilities usage

### 📝 [Custom Logging](LOGGING.md)

Integration guide for popular Go logging libraries with KongTask.

**Contents:**

- Logger interface implementation
- Logrus integration
- Zap integration
- Zerolog integration
- Async logging patterns
- Log filtering and redaction
- Performance considerations
- Production logging strategies

### 🚀 [Deployment](DEPLOYMENT.md)

Production deployment and operations guide for KongTask.

**Contents:**

- Production deployment strategies
- Database configuration and tuning
- Docker containerization
- Kubernetes deployment
- Monitoring and observability
- Health checks and metrics
- Performance optimization
- Security best practices
- Troubleshooting guide

## Getting Started

1. **Start with the main [README.md](../README.md)** for a quick overview and basic usage
2. **Review the [API Reference](API_REFERENCE.md)** to understand available functions and types
3. **Follow the [Task Handlers Guide](TASK_HANDLERS.md)** to implement your job processing logic
4. **Configure cron scheduling** using the [Crontab Guide](CRONTAB.md) if needed
5. **Set up custom logging** with the [Logging Guide](LOGGING.md)
6. **Deploy to production** following the [Deployment Guide](DEPLOYMENT.md)

## Quick Navigation

### Common Tasks

- [Creating task handlers](TASK_HANDLERS.md#basic-task-handler)
- [Adding jobs to the queue](API_REFERENCE.md#adding-jobs)
- [Setting up cron jobs](CRONTAB.md#basic-cron-configuration)
- [Configuring logging](LOGGING.md#logrus-integration)
- [Production deployment](DEPLOYMENT.md#production-deployment)

### Advanced Features

- [Job key modes and deduplication](API_REFERENCE.md#job-specifications)
- [Forbidden flags for job filtering](API_REFERENCE.md#administrative-functions)
- [Event system for monitoring](API_REFERENCE.md#events)
- [Custom worker configurations](API_REFERENCE.md#worker-types)
- [Performance tuning](DEPLOYMENT.md#performance-optimization)

### Integration Examples

- [Basic worker setup](API_REFERENCE.md#basic-worker-setup)
- [Advanced job scheduling](API_REFERENCE.md#advanced-job-scheduling)
- [Event monitoring](API_REFERENCE.md#event-monitoring)
- [Custom logger integration](API_REFERENCE.md#custom-logger-integration)
- [Docker deployment](DEPLOYMENT.md#docker-deployment)

## Contributing to Documentation

When contributing to KongTask documentation:

1. **Keep examples practical** - Use real-world scenarios in code examples
2. **Maintain consistency** - Follow the established formatting and style
3. **Update cross-references** - Ensure links between documents remain accurate
4. **Test code examples** - Verify all code snippets compile and work correctly
5. **Update the index** - Add new sections to this index file

## Support

If you find issues with the documentation or need clarification:

1. Check the [main README.md](../README.md) for basic information
2. Search through the relevant documentation files
3. Open an issue on the GitHub repository
4. Contribute improvements via pull requests

The documentation is continuously updated to reflect the latest features and best practices for KongTask.
