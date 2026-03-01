# Start dark-factory (watches prompts/queue/ for work)
.PHONY: run
run:
	dark-factory

# Show current status
.PHONY: status
status:
	dark-factory status

# Move all prompts from inbox to queue
.PHONY: queue
queue:
	@for f in prompts/*.md; do \
		[ -f "$$f" ] && mv "$$f" prompts/queue/ && echo "queued: $$(basename $$f)"; \
	done || echo "no prompts to queue"
