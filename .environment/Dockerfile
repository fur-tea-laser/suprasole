FROM codercom/code-server:latest

USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    unzip \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# 1. Go: unpack into /usr/local and expose binaries
RUN curl -fsSL https://go.dev/dl/go1.27.1.linux-amd64.tar.gz | tar -C /usr/local -xzf - && \
    ln -s /usr/local/go/bin/* /usr/local/bin/

# 2. Deno: official system-wide install flag
RUN curl -fsSL https://deno.land/install.sh | DENO_INSTALL=/usr/local sh

# 3. Antigravity CLI: install as root and promote directly to /usr/local/bin
RUN curl -fsSL https://antigravity.google/cli/install.sh | bash && \
    mv /root/.local/bin/agy /usr/local/bin/

USER coder