FROM ubuntu:22.04

# Avoid interactive prompts and set up non-root user if needed
ENV DEBIAN_FRONTEND=noninteractive
ENV NG_CLI_ANALYTICS=false
ENV CI=true

# Install curl, Node.js 18.x, and Angular CLI
RUN apt-get update \
    && apt-get install -y curl \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && npm install -g @angular/cli \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Optionally, set workdir for your workspace
WORKDIR /workspace

# Default command (override as needed)
CMD [ "bash" ]