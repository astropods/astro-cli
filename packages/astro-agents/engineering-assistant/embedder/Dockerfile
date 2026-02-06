FROM python:3.11-slim

WORKDIR /app

# Install wget for healthcheck
RUN apt-get update && apt-get install -y wget && rm -rf /var/lib/apt/lists/*

# Use fastembed (ONNX-based, no torch dependency) to keep image small
RUN pip install --no-cache-dir \
    fastembed \
    flask==3.0.0

# Copy server
COPY server.py .

EXPOSE 8000

CMD ["python", "server.py"]
