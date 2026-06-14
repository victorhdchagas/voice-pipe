package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ── Config ──────────────────────────────────────────────────────────────

var (
	port         = envDefault("PORT", "8646")
	vmToken      = os.Getenv("VM_TOKEN")
	vmDevice     = envDefault("VM_DEVICE", "cozinha")
	vmChime      = envDefault("VM_CHIME", "soundbank://soundlibrary/alarms/beeps_and_bloops/intro_02")
	vmLanguage   = envDefault("VM_LANGUAGE", "pt-BR")
	vmBaseURL    = envDefault("VM_BASE_URL", "https://api-v2.voicemonkey.io/announcement")
	startedAt    = time.Now()
	version      = "2.0.0"
	soundsDir    = "/hermes-data/sounds/downloaded"
	soundsJSON   = "/hermes-data/sounds/ask-soundlibrary.json"
)

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ── Sound Library JSON ─────────────────────────────────────────────────

type SoundLibrary struct {
	BaseURL  BaseURL `json:"baseUrl"`
	FileType string  `json:"defaultFileType"`
	Sounds   []Sound `json:"sounds"`
}

type BaseURL struct {
	CloudFront  string `json:"cloudFrontUrl"`
	SoundBank   string `json:"soundBankUrl"`
}

type Sound struct {
	AudioPath string  `json:"audioFilePath"`
	Category  string  `json:"category"`
	Duration  float64 `json:"duration"`
	Name      string  `json:"name"`
}

var (
	lib      *SoundLibrary
	catOrder []string
	catMap   map[string][]Sound
	soundsPageHTML string // pre-rendered HTML
)

func loadSoundLibrary() error {
	raw, err := os.ReadFile(soundsJSON)
	if err != nil {
		return fmt.Errorf("lendo %s: %w", soundsJSON, err)
	}
	var l SoundLibrary
	if err := json.Unmarshal(raw, &l); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}
	lib = &l

	// Group by category
	catMap = make(map[string][]Sound)
	for _, s := range l.Sounds {
		catMap[s.Category] = append(catMap[s.Category], s)
	}

	// Sort categories
	catOrder = make([]string, 0, len(catMap))
	for c := range catMap {
		catOrder = append(catOrder, c)
	}
	sort.Strings(catOrder)

	log.Printf("biblioteca carregada: %d sons em %d categorias", len(l.Sounds), len(catOrder))

	// Pré-renderizar página /sounds (cache estático)
	soundsPageHTML = renderSoundsPage()
	return nil
}

// renderSoundsPage gera o HTML completo da página /sounds (executado 1x na inicialização)
func renderSoundsPage() string {
	var catHTML strings.Builder
	for _, cat := range catOrder {
		sounds := catMap[cat]
		catID := strings.ReplaceAll(strings.ToLower(cat), "/", "-")
		catID = strings.ReplaceAll(catID, "_", "-")

		catHTML.WriteString(fmt.Sprintf(`
<details>
<summary class="cat-header">
  <span class="cat-name">%s</span>
  <span class="cat-count">%d sons</span>
</summary>
<div class="cat-sounds" id="cat-%s">
<table>
<tr><th>Som</th><th>Duração</th><th>Player</th><th>Alexa</th></tr>
`, escapeHTML(cat), len(sounds), catID))

		for _, s := range sounds {
			audioURL := fmt.Sprintf("/audio/%s.%s", s.AudioPath, lib.FileType)
			dur := fmt.Sprintf("%.1fs", s.Duration)
			soundbankURL := lib.BaseURL.SoundBank + s.AudioPath

			btnTest := fmt.Sprintf(`<button class="btn" onclick="testar('%s', this)" title="Tocar na Alexa">🔊 Testar</button>`, escapeJSStr(soundbankURL))
			player := fmt.Sprintf(`<audio controls preload="none"><source src="%s" type="audio/mpeg"></audio>`, escapeHTML(audioURL))

			catHTML.WriteString(fmt.Sprintf(`<tr><td><strong>%s</strong></td><td>%s</td><td>%s</td><td>%s</td></tr>
`, escapeHTML(s.Name), dur, player, btnTest))
		}

		catHTML.WriteString(`</table></div></details>`)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>voice-pipe — Biblioteca de Sons</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
         max-width: 960px; margin: 20px auto; padding: 0 16px;
         background: #0d1117; color: #c9d1d9; line-height: 1.5; font-size: 14px; }
  h1 { color: #58a6ff; font-size: 1.5em; display: flex; align-items: center; gap: 12px; }
  h1 small { font-size: 0.6em; color: #8b949e; font-weight: normal; }
  a { color: #58a6ff; }
  a:hover { text-decoration: underline; }

  .cat-header {
    background: #161b22; padding: 10px 16px; border-radius: 8px;
    cursor: pointer; display: flex; align-items: center; gap: 12px;
    margin: 4px 0; user-select: none; font-size: 14px; font-weight: 600;
    border: 1px solid #30363d; transition: background 0.15s;
  }
  .cat-header:hover { background: #1c2333; }
  .cat-name { flex: 1; color: #e6edf3; }
  .cat-count { color: #8b949e; font-size: 0.85em; font-weight: normal; }
  details[open] .cat-header { border-radius: 8px 8px 0 0; border-bottom: none; }

  table { width: 100%%; border-collapse: collapse; }
  th, td { padding: 6px 10px; text-align: left; border-bottom: 1px solid #21262d;
           vertical-align: middle; }
  th { color: #8b949e; font-weight: 600; font-size: 0.8em; text-transform: uppercase;
       background: #0d1117; position: sticky; top: 0; }
  tr:hover { background: #161b22; }
  tr:last-child td { border-bottom: none; }

  .cat-sounds { background: #0d1117; border: 1px solid #30363d; border-top: none;
                border-radius: 0 0 8px 8px; overflow: hidden; }

  .btn { background: #1f6feb; color: #fff; border: none; padding: 4px 10px;
         border-radius: 6px; cursor: pointer; font-size: 0.85em;
         white-space: nowrap; transition: background 0.15s; }
  .btn:hover { background: #388bfd; }
  .btn:disabled { opacity: 0.5; cursor: wait; }
  .btn.ok { background: #238636; }
  .btn.err { background: #da3633; }

  audio { height: 32px; width: 180px; }
  audio::-webkit-media-controls-panel { background: #161b22; }

  hr { border: none; border-top: 1px solid #30363d; margin: 20px 0; }

  .search-box {
    width: 100%%; padding: 10px 14px; border-radius: 8px; border: 1px solid #30363d;
    background: #161b22; color: #c9d1d9; font-size: 14px; margin: 8px 0 16px;
    box-sizing: border-box;
  }
  .search-box:focus { outline: none; border-color: #388bfd; }

  .stats { display: flex; gap: 16px; flex-wrap: wrap; margin: 12px 0; }
  .stat { background: #161b22; padding: 6px 14px; border-radius: 6px;
           border: 1px solid #30363d; font-size: 0.85em; color: #8b949e; }
  .stat strong { color: #e6edf3; }

  @media (max-width: 700px) {
    body { font-size: 13px; padding: 0 8px; }
    audio { width: 120px; }
    .cat-header { font-size: 13px; }
  }
</style>
</head>
<body>
<h1>🔊 Biblioteca de Sons <small>voice-pipe v%s</small></h1>
<p><a href="/" style="color:#8b949e;">← Voltar</a></p>

<div class="stats">
  <span class="stat">📁 <strong>%d</strong> categorias</span>
  <span class="stat">🎵 <strong>%d</strong> sons</span>
</div>

<input type="text" class="search-box" id="search" placeholder="Buscar sons..." oninput="filterSounds()">

<div id="sounds-container">
%s
</div>

<script>
function testar(value, btn) {
  btn.disabled = true;
  btn.textContent = '⏳';
  btn.className = 'btn';
  fetch('/speak', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({ text: 'Teste de som', chime: value || undefined })
  })
  .then(r => r.json())
  .then(data => {
    if (data.success) { btn.textContent = '✅'; btn.className = 'btn ok'; }
    else { btn.textContent = '❌'; btn.className = 'btn err'; }
    setTimeout(() => { btn.textContent = '🔊 Testar'; btn.className = 'btn'; btn.disabled = false; }, 2500);
  })
  .catch(() => {
    btn.textContent = '❌'; btn.className = 'btn err';
    setTimeout(() => { btn.textContent = '🔊 Testar'; btn.className = 'btn'; btn.disabled = false; }, 2500);
  });
}

function filterSounds() {
  const q = document.getElementById('search').value.toLowerCase();
  const details = document.querySelectorAll('details');
  details.forEach(d => {
    const rows = d.querySelectorAll('tr:not(:first-child)');
    let hasMatch = false;
    rows.forEach(row => {
      const text = row.textContent.toLowerCase();
      if (text.includes(q)) {
        row.style.display = '';
        hasMatch = true;
      } else {
        row.style.display = 'none';
      }
    });
    if (q === '' || hasMatch) {
      d.style.display = '';
      if (q !== '' && hasMatch) d.open = true;
    } else {
      d.style.display = 'none';
    }
  });
}
</script>

<hr>
<p style="color:#8b949e;font-size:0.85em;">
  🔊 voice-pipe — Sons disponíveis localmente
</p>
</body>
</html>`,
		version, len(catOrder), len(lib.Sounds), catHTML.String())
}

// ── Voice Monkey ───────────────────────────────────────────────────────

type SpeakRequest struct {
	Text   string `json:"text"`
	Device string `json:"device,omitempty"`
	Chime  string `json:"chime,omitempty"`
}

type SpeakResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func callVoiceMonkey(text, device, chime string) error {
	params := url.Values{
		"token":    {vmToken},
		"device":   {device},
		"text":     {text},
		"language": {vmLanguage},
	}
	if chime != "" {
		params.Set("chime", chime)
	}

	fullURL := fmt.Sprintf("%s?%s", vmBaseURL, params.Encode())

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(fullURL)
	if err != nil {
		return fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Voice Monkey retornou %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── Handlers ───────────────────────────────────────────────────────────

func handleSpeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"método não permitido"}`, http.StatusMethodNotAllowed)
		return
	}

	var req SpeakRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"JSON inválido"}`, http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, `{"error":"campo 'text' é obrigatório"}`, http.StatusBadRequest)
		return
	}
	if req.Device == "" {
		req.Device = vmDevice
	}

	log.Printf("[voice-pipe] falando '%s' em '%s'", req.Text[:min(len(req.Text), 60)], req.Device)

	if err := callVoiceMonkey(req.Text, req.Device, req.Chime); err != nil {
		log.Printf("[voice-pipe] erro: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(SpeakResponse{Success: false, Message: err.Error()})
		return
	}

	log.Printf("[voice-pipe] ok")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SpeakResponse{Success: true, Message: "Alexa vai falar agora"})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleSounds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(soundsPageHTML))
}

func handleSoundsJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(lib)
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}

func escapeJSStr(s string) string {
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	tokenDisplay := "✓ configurado"
	if vmToken == "" {
		tokenDisplay = "❌ não configurado"
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>voice-pipe</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
         max-width: 680px; margin: 40px auto; padding: 0 20px;
         background: #0d1117; color: #c9d1d9; line-height: 1.6; }
  h1 { color: #58a6ff; }
  code { background: #161b22; padding: 2px 6px; border-radius: 4px; font-size: 0.9em; }
  pre { background: #161b22; padding: 16px; border-radius: 8px; overflow-x: auto; }
  .badge { display: inline-block; padding: 2px 10px; border-radius: 12px; font-size: 0.8em; font-weight: 600; }
  .ok { background: #238636; color: #fff; }
  .warn { background: #d29922; color: #fff; }
  hr { border: none; border-top: 1px solid #30363d; margin: 24px 0; }
  .feature-list { list-style: none; padding: 0; }
  .feature-list li { padding: 6px 0; }
  .feature-list li::before { content: "✨ "; }
</style>
</head>
<body>
<h1>🔊 voice-pipe</h1>
<p><span class="badge ok">online</span> v%s</p>
<p>
  Ponte HTTP para o <strong>Voice Monkey</strong> + <strong>Biblioteca de Sons Alexa</strong>.
</p>

<ul class="feature-list">
  <li>TTS na Alexa via POST /speak</li>
  <li><a href="/sounds" style="color: #58a6ff;">🎵 Biblioteca com %d sons</a> — player no navegador + Alexa</li>
</ul>

<hr>
<h2>📊 Status</h2>
<p>Token: %s</p>
<p>Dispositivo padrão: <code>%s</code></p>
<p>Idioma: <code>%s</code></p>
<p>Uptime: %s</p>

<hr>

<h2>📥 Como usar</h2>
<pre>
curl -X POST http://localhost:%s/speak \
  -H "Content-Type: application/json" \
  -d '{"text": "Olá, isso é um teste"}'
</pre>
<p>Para tocar um som antes de falar:</p>
<pre>
curl -X POST http://localhost:%s/speak \
  -H "Content-Type: application/json" \
  -d '{"text": "Alerta", "chime": "soundbank://soundlibrary/alarms/buzzers/buzzers_01"}'
</pre>

<hr>
<p style="color: #8b949e; font-size: 0.85em;">
  voice-pipe &mdash; Alexa TTS bridge via Voice Monkey
</p>
</body>
</html>`,
		version, len(lib.Sounds), tokenDisplay, vmDevice, vmLanguage,
		time.Since(startedAt).Round(time.Second),
		port, port)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// ── Audio file server ──────────────────────────────────────────────────

func handleAudio(w http.ResponseWriter, r *http.Request) {
	// /audio/air/fire_extinguisher/fire_extinguisher_01.mp3
	relPath := strings.TrimPrefix(r.URL.Path, "/audio/")
	if relPath == "" || relPath == r.URL.Path {
		http.NotFound(w, r)
		return
	}

	// Security: prevent directory traversal
	fullPath := filepath.Join(soundsDir, relPath)
	if !strings.HasPrefix(fullPath, soundsDir) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	// Check file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, fullPath)
}

// ── Main ───────────────────────────────────────────────────────────────

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[voice-pipe] ")

	if vmToken == "" {
		log.Fatal("VM_TOKEN não configurado no .env")
	}

	if err := loadSoundLibrary(); err != nil {
		log.Fatalf("erro ao carregar biblioteca de sons: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/sounds", handleSounds)
	mux.HandleFunc("/sounds.json", handleSoundsJSON)
	mux.HandleFunc("/speak", handleSpeak)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/audio/", handleAudio)

	addr := fmt.Sprintf("0.0.0.0:%s", port)
	log.Printf("iniciado — ouvindo em %s", addr)
	log.Printf("dispositivo padrão: %s", vmDevice)
	log.Printf("biblioteca: %d sons em %d categorias", len(lib.Sounds), len(catOrder))

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("erro: %v", err)
	}
}
