FROM ubuntu:22.04

# Avoid interactive prompts and set up non-root user if needed
ENV DEBIAN_FRONTEND=noninteractive
ENV NG_CLI_ANALYTICS=false
ENV CI=true



# Install curl, Node.js 18.x, and Angular CLI
RUN apt-get update \
    && apt-get install -y curl tree \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs git \
    && npm install -g @angular/cli \
    && apt-get clean \
    && apt install -y git \
    && ng new user-app --defaults --no-interactive --style=scss \
    && cd user-app \
    && npm i --save @angular/material @angular/cdk @angular/animations \
    && npm i --save @ngx-translate/core @ngx-translate/http-loader \
    && ng add @angular/material --defaults --no-interactive || true \
    && rm -rf /var/lib/apt/lists/*

# Optionally, set workdir for your workspace
WORKDIR /user-app

# Default command (override as needed)
CMD [ "bash" ]