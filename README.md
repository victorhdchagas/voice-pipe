# voice-pipe

Ponte HTTP entre Hermes Agent e **Voice Monkey** (Alexa TTS).

Recebe texto via POST e faz a Alexa falar em qualquer dispositivo Echo.

## Arquitetura

```
curl / Hermes / alert-proxy
  │ POST http://localhost:8646/speak
  ▼
voice-pipe (Go, ~1MB RAM)
  │ GET  https://api-v2.voicemonkey.io/announcement
  ▼
Alexa 🔊
```

## Config (.env)

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `PORT` | `8646` | Porta do servidor |
| `VM_TOKEN` | — | **Obrigatório.** Token da API Voice Monkey |
| `VM_DEVICE` | `cozinha` | Dispositivo Alexa padrão |
| `VM_LANGUAGE` | `pt-BR` | Idioma do TTS |
| `VM_BASE_URL` | `https://api-v2.voicemonkey.io/announcement` | URL da API |

## API

### POST /speak

```json
{
  "text": "Olá, isso é um teste",
  "device": "cozinha",
  "chime": "soundbank://soundlibrary/alarms/beeps_and_bloops/intro_02"
}
```

| Campo | Obrigatório | Descrição |
|-------|-------------|-----------|
| `text` | ✅ | Texto que a Alexa vai falar |
| `device` | ❌ | Dispositivo (padrão: `VM_DEVICE`) |
| `chime` | ❌ | Som de alerta (ver `/sounds` para lista) |

### GET / — Página inicial com instruções
### GET /sounds — Lista de sons com botões para testar

## Service (systemd)

```bash
sudo systemctl enable --now voice-pipe
sudo systemctl status voice-pipe
journalctl -u voice-pipe -n 20
```

## Paths

| Item | Caminho |
|------|---------|
| Binário | `/hermes-data/voice-pipe/voice-pipe` |
| Código | `/hermes-data/voice-pipe/main.go` |
| Config | `/hermes-data/voice-pipe/.env` |
| Service | `/etc/systemd/system/voice-pipe.service` |
| Logs | `journalctl -u voice-pipe` |
| Página web | `http://<host>:8646/` |
