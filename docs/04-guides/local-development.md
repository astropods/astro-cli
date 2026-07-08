# Local Development Runbook

A runbook for developing and testing agents against a locally running Astro platform.

---

## 1. Building the local CLI (`ast-dev`)

The development CLI is called `ast-dev`. It's built from the monorepo with a different binary name and defaults — most importantly, it points at `localhost:8080` for the server and auto-configures local push behavior.

```bash
moon run astro-cli:link
```

This builds `ast-dev` and symlinks it into `$HOME/go/bin/`. Make sure your Go bin is on your PATH:

```bash
# Add to your ~/.zshrc or ~/.bashrc
export PATH="$HOME/go/bin:$PATH"
```

Verify it works:

```bash
ast-dev version
```

To unlink later:

```bash
moon run astro-cli:unlink
```

---

## 2. `ast-dev` vs `ast`

`ast-dev` is **only for use with a locally running astro-server**. Key differences:

| | `ast` (production) | `ast-dev` (local) |
|---|---|---|
| Server URL | `https://astropod.ai` | `http://localhost:8080` |
| Registry URL | `registry.astropod.ai` | `registry.localhost` |
| `push` behavior | Pushes to remote registry | Skips push, retags locally |
| `push` platform | `linux/amd64` | Native (your machine's arch) |

Do not use `ast-dev` against production. Do not use `ast` against a local server (unless you pass `--server`).

---

## 3. Running an agent locally

### Without `--local` (default)

Everything runs in Docker — agent included.

```bash
cd my-agent
ast-dev dev                    # start all containers in background
ast-dev dev logs               # tail agent logs
ast-dev dev logs --all         # tail all service logs
ast-dev dev stop               # tear down
```

**When to use:** You're developing your agent code and don't need to modify platform packages.

### With `--local`

The agent runs as a **host process** with hot-reload. Infrastructure services (messaging) still run in Docker.

```bash
export ASTRO_ROOT=$HOME/code/astro    # path to astro monorepo
cd my-agent
ast-dev dev --local                    # blocks until Ctrl+C
```

**What it does:**
- Removes the agent container from Docker Compose
- Symlinks `@astropods/*` packages from `ASTRO_ROOT` into `node_modules` (TS) or pip-installs them editable (Py)
- Runs the agent via `bun --watch run start` (TS) or `python3 -m agent.main` (Py)
- Rewrites Docker service hostnames to `localhost` in the agent env
- Streams Docker Compose logs alongside the agent process

**When to use:** You're modifying platform SDKs (`@astropods/messaging`, `@astropods/adapter-core`, etc.) and want changes reflected immediately in the agent.

**Cleaning up** after `--local`:

```bash
ast-dev dev --local-reset      # removes symlinked packages
bun install                    # restore normal deps (TS)
# or
pip install -r requirements.txt  # restore normal deps (Py)
```

---

## 4. Building and using custom infrastructure images

Use `moon run deployment:<target>` to build infrastructure images from source:

```bash
moon run deployment:messaging      # builds messaging:latest (includes playground UI)
```

Then override the default image in your agent's `astropods.yml`:

```yaml
dev:
  overrides:
    messagingImage: "messaging:latest"
```

When an override is set, the CLI uses that image directly and skips pulling from the remote registry.

**Example — testing a messaging change:**

```bash
# 1. Make your changes to modules/messaging/
# 2. Build the image
moon run deployment:messaging

# 3. Override in your agent spec
# astropods.yml:
#   dev:
#     overrides:
#       messagingImage: "messaging:latest"

# 4. Run the agent
ast-dev dev
```

---

## 5. Pushing to a local server

With `ast-dev`, `push` automatically skips the remote registry and retags images so the local astro-server can resolve them:

```bash
ast-dev push                   # build + retag + register with localhost:8080
```

This is equivalent to running `ast push --skip-push --platform <native>` — the images are tagged with registry-qualified names (`registry.localhost/<namespace>/<agent>:<tag>`) so K8s with `imagePullPolicy: IfNotPresent` can resolve them.

### Infrastructure images for local K8s

The collector and messaging sidecars are not part of `ast dev` compose — they run as K8s sidecars. When deploying to a local K8s cluster, you need to build and tag them with the names the server expects (both use Docker Hub names):

```bash
moon run deployment:collector    # builds collector:latest + tags astropods/collector:latest
moon run deployment:messaging    # builds messaging:latest + tags astropods/messaging:latest
```

Without these, the sidecar pods will fail to pull their images.

#### Reloading a rebuilt sidecar image (Docker Desktop / kind)

Building the image is not always enough. Recent Docker Desktop runs Kubernetes on
a node (`desktop-control-plane`) whose **containerd image store is separate from
the Docker daemon**, so a `docker build` is invisible to the cluster. The sidecars
deploy with `imagePullPolicy: IfNotPresent`, so if containerd already has an
`astropods/messaging:latest` (e.g. a stale Docker Hub pull), the cluster keeps
using it and your rebuild never takes effect — the pod silently runs old code.

Symptoms of a stale sidecar: no `chat.db` under `/data`, no `Chat store
initialized` log line, and empty chat history despite `CHAT_DB_PATH` being set.

After rebuilding, load the image into the cluster's containerd and restart the
pod so `IfNotPresent` re-resolves the tag:

```bash
moon run deployment:messaging

# Import the freshly built image into the cluster's containerd (k8s.io namespace)
docker save astropods/messaging:latest \
  | docker exec -i desktop-control-plane ctr -n k8s.io images import -

# Restart the agent so the StatefulSet recreates its pod on the new image
kubectl -n <agent-namespace> delete pod <agent-pod>
```

Verify the reload took effect:

```bash
# New chat-store line appears and chat.db is created eagerly at startup
kubectl -n <agent-namespace> logs <agent-pod> -c messaging | grep "Chat store initialized"
kubectl -n <agent-namespace> exec <agent-pod> -c messaging -- ls -la /data
```

The messaging sidecar runs as a native sidecar (an init container with
`restartPolicy: Always`), so target it with `-c messaging` and expect it in
`.spec.initContainers`, not `.spec.containers`. The same reload applies to
`astropods/collector:latest`.

---

## 6. Example scenarios

### "I'm building a new agent from scratch"

```bash
moon run astro-cli:link                # build ast-dev (first time only)
ast-dev create my-agent                # scaffold
cd my-agent
ast-dev configure                      # set API keys
ast-dev dev                            # start everything in Docker
ast-dev dev logs                       # watch agent output
# ... edit agent/index.ts, rebuild ...
ast-dev dev stop
ast-dev dev                            # restart with changes
```

### "I want to deploy my agent to a local K8s cluster"

```bash
# 1. Build infrastructure sidecar images (auto-tags astropods/* names)
moon run deployment:collector
moon run deployment:messaging

# 2. Push agent to local server
cd my-agent
ast-dev login                          # authenticate with local server
ast-dev push                           # build + retag + register
# astro-server picks up the spec and creates the K8s deployment
# images resolve locally via registry.localhost/... tags
```

### "I'm debugging the messaging sidecar"

```bash
# 1. Make changes to modules/messaging/
# 2. Build the image
moon run deployment:messaging

# 3. Point your agent at it
# astropods.yml:
#   dev:
#     overrides:
#       messagingImage: "messaging:latest"

# 4. Run
cd my-agent
ast-dev dev
ast-dev dev logs astro-messaging       # tail just the messaging container
```

### "I'm changing the `@astropods/messaging` SDK and testing with an agent"

```bash
export ASTRO_ROOT=$HOME/code/astro
cd my-agent
ast-dev dev --local                    # agent runs on host with symlinked SDK
# ... edit modules/messaging/sdk/node/src/*, changes reflect immediately ...
# Ctrl+C to stop
ast-dev dev --local-reset              # clean up symlinks
bun install                            # restore normal deps
```

### "I want to see what compose config my spec generates without starting anything"

```bash
cd my-agent
ast-dev explain
```

### "I'm done and want to clean up everything"

```bash
ast-dev dev stop                       # stop running containers
ast-dev dev --local-reset              # if you used --local
moon run deployment:clean              # remove local Docker images
moon run astro-cli:unlink              # remove ast-dev from PATH
```

