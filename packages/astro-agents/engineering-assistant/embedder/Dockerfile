FROM python:3.11-slim

WORKDIR /app

# Install wget for healthcheck
RUN apt-get update && apt-get install -y wget && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -g 1001 astro && \
    useradd -u 1001 -g astro -s /bin/sh astro

# Use fastembed (ONNX-based, no torch dependency) to keep image small
RUN pip install --no-cache-dir \
    fastembed \
    flask==3.0.0

# Copy server
COPY server.py .

# Set ownership and switch to non-root user
RUN chown -R astro:astro /app
USER astro

EXPOSE 8000

CMD ["python", "server.py"]
