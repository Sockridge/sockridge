import base64
import json
import os
from pathlib import Path
from cryptography.hazmat.primitives.asymmetric.ed25519 import (
    Ed25519PrivateKey,
    Ed25519PublicKey,
)
from cryptography.hazmat.primitives.serialization import (
    Encoding,
    PublicFormat,
    PrivateFormat,
    NoEncryption,
)


class KeyPair:
    def __init__(self, private_key: Ed25519PrivateKey):
        self._private_key = private_key
        self._public_key  = private_key.public_key()

    @classmethod
    def generate(cls) -> "KeyPair":
        """Generate a new Ed25519 keypair."""
        return cls(Ed25519PrivateKey.generate())

    @classmethod
    def load(cls, path: str) -> "KeyPair":
        """Load a private key from a base64-encoded file."""
        raw = Path(path).read_text().strip()
        priv_bytes = base64.b64decode(raw)
        private_key = Ed25519PrivateKey.from_private_bytes(priv_bytes[:32])
        return cls(private_key)

    def save(self, path: str) -> None:
        """Save the private key to a file (chmod 600)."""
        priv_bytes = self._private_key.private_bytes(
            Encoding.Raw, PrivateFormat.Raw, NoEncryption()
        )
        Path(path).write_text(base64.b64encode(priv_bytes).decode())
        os.chmod(path, 0o600)

    @property
    def public_key_b64(self) -> str:
        """Base64-encoded public key for registration."""
        pub_bytes = self._public_key.public_bytes(Encoding.Raw, PublicFormat.Raw)
        return base64.b64encode(pub_bytes).decode()

    def sign(self, message: bytes) -> bytes:
        """Sign a message with the private key."""
        return self._private_key.sign(message)


class Credentials:
    DEFAULT_PATH = Path.home() / ".sockridge" / "credentials.json"
    DEFAULT_KEY  = Path.home() / ".sockridge" / "ed25519.key"

    def __init__(
        self,
        publisher_id: str,
        handle: str,
        server_url: str,
        session_token: str = "",
    ):
        self.publisher_id  = publisher_id
        self.handle        = handle
        self.server_url    = server_url
        self.session_token = session_token

    @classmethod
    def load(cls, path: str | None = None) -> "Credentials":
        p = Path(path) if path else cls.DEFAULT_PATH
        data = json.loads(p.read_text())
        return cls(
            publisher_id  = data["publisher_id"],
            handle        = data["handle"],
            server_url    = data.get("server_url", "https://sockridge.com:9000"),
            session_token = data.get("session_token", ""),
        )

    def save(self, path: str | None = None) -> None:
        p = Path(path) if path else cls.DEFAULT_PATH
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(json.dumps({
            "publisher_id":  self.publisher_id,
            "handle":        self.handle,
            "server_url":    self.server_url,
            "session_token": self.session_token,
        }, indent=2))
        os.chmod(p, 0o600)
