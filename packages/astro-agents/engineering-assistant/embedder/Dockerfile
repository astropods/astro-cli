FROM python:3.11-slim

WORKDIR /app

# Install dependencies with compatible versions
RUN pip install --no-cache-dir \
    "numpy<2" \
    torch==2.5.1 --index-url https://download.pytorch.org/whl/cpu && \
    pip install --no-cache-dir \
    sentence-transformers==3.3.1 \
    flask==3.0.0

# Copy server
COPY server.py .

EXPOSE 8000

CMD ["python", "server.py"]
