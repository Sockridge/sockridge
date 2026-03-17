from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer
import uvicorn
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="embedder")

# load model once at startup — all-MiniLM-L6-v2 is 384-dim, fast, good quality
model = SentenceTransformer("sentence-transformers/all-MiniLM-L6-v2")
logger.info("model loaded: all-MiniLM-L6-v2")


class EmbedRequest(BaseModel):
    texts: list[str]  # batch of strings to embed


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]  # one 384-dim vector per input text


@app.post("/embed", response_model=EmbedResponse)
def embed(req: EmbedRequest):
    if not req.texts:
        raise HTTPException(status_code=400, detail="texts cannot be empty")
    if len(req.texts) > 64:
        raise HTTPException(status_code=400, detail="max 64 texts per request")

    vecs = model.encode(req.texts, normalize_embeddings=True).tolist()
    return EmbedResponse(embeddings=vecs)


@app.get("/healthz")
def health():
    return {"status": "ok"}


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
