FROM codercom/code-server:latest

USER root

RUN apt-get update && apt-get install -y \
    curl \
    unzip \
    && rm -rf /var/lib/apt/lists/*

ENV HOME="/home/coder"

# Install Go toolchain and configure path
RUN curl -fsSL https://go.dev/dl/go1.22.5.linux-amd64.tar.gz | tar -C /usr/local -xzf - && \
    echo "export PATH=\"/usr/local/go/bin:\$HOME/go/bin:\$PATH\"" >> $HOME/.bashrc

# Install Deno and configure path
RUN curl -fsSL https://deno.land/install.sh | sh && \
    echo "export PATH=\"\$HOME/.deno/bin:\$PATH\"" >> $HOME/.bashrc

# Fix permissions
RUN chown -R coder:coder $HOME/.bashrc $HOME/.deno

USER coder

# Install Antigravity CLI (agy) and update user PATH
RUN curl -fsSL https://antigravity.google/cli/install.sh | bash && \
    echo "export PATH=\"\$HOME/.local/bin:\$PATH\"" >> $HOME/.bashrc
