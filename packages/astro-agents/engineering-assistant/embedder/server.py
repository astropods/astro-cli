from flask import Flask, request, jsonify
from sentence_transformers import SentenceTransformer
import logging

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)

# Load model on startup
model = SentenceTransformer('all-MiniLM-L6-v2')
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
    embeddings = model.encode(texts)

    return jsonify({
        "embeddings": embeddings.tolist(),
        "model": "all-MiniLM-L6-v2",
        "dimensions": 384
    })

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8000)
