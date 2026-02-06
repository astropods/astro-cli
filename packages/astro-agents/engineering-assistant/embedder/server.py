from flask import Flask, request, jsonify
from fastembed import TextEmbedding
import logging

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)

# Load model on startup
model = TextEmbedding('sentence-transformers/all-MiniLM-L6-v2')
logging.info("Embedding model loaded successfully")

@app.route('/health', methods=['GET'])
def health():
    return jsonify({"status": "healthy", "model": "all-MiniLM-L6-v2"})

@app.route('/embed', methods=['POST'])
def embed():
    data = request.json
    texts = data.get('texts', [])

    if not texts:
        return jsonify({"error": "No texts provided"}), 400

    # Generate embeddings
    embeddings = list(model.embed(texts))

    return jsonify({
        "embeddings": [e.tolist() for e in embeddings],
        "model": "all-MiniLM-L6-v2",
        "dimensions": 384
    })

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8000)
