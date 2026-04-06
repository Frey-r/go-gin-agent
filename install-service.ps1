# ============================================================
# install-service.ps1 — Instala go-gin-agent como servicio de Windows usando NSSM
# ============================================================
# Ejecutar como Administrador:
#   powershell -ExecutionPolicy Bypass -File .\install-service.ps1
# ============================================================

$ServiceName   = "go-gin-agent"
$ProjectDir    = Split-Path -Parent $MyInvocation.MyCommand.Definition
$ExePath       = Join-Path $ProjectDir "go-gin-agent.exe"
$LogDir        = Join-Path $ProjectDir "logs"
$EnvFile       = Join-Path $ProjectDir ".env"

# ── Verificar que existe el binario ──────────────────────────
if (-not (Test-Path $ExePath)) {
    Write-Error "No se encontró el binario: $ExePath"
    Write-Host "Compilar primero con: go build -ldflags='-s -w' -o go-gin-agent.exe ./cmd/server"
    exit 1
}

# ── Verificar que existe .env ────────────────────────────────
if (-not (Test-Path $EnvFile)) {
    Write-Error "No se encontró .env en $EnvFile"
    Write-Host "Copiar .env.example a .env y configurar las variables."
    exit 1
}

# ── Crear directorio de logs ─────────────────────────────────
if (-not (Test-Path $LogDir)) {
    New-Item -ItemType Directory -Path $LogDir | Out-Null
    Write-Host "[OK] Directorio de logs creado: $LogDir"
}

# ── Verificar NSSM ──────────────────────────────────────────
$nssm = Get-Command nssm -ErrorAction SilentlyContinue
if (-not $nssm) {
    Write-Host ""
    Write-Host "NSSM no está instalado. Instalar con:"
    Write-Host "  winget install nssm"
    Write-Host "  -- o --"
    Write-Host "  choco install nssm"
    Write-Host "  -- o --"
    Write-Host "  Descargar de https://nssm.cc/download"
    Write-Host ""
    exit 1
}

# ── Remover servicio previo si existe ────────────────────────
$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existing) {
    Write-Host "Servicio existente encontrado, removiendo..."
    nssm stop $ServiceName confirm
    nssm remove $ServiceName confirm
    Start-Sleep -Seconds 2
}

# ── Instalar servicio ───────────────────────────────────────
Write-Host "Instalando servicio '$ServiceName'..."
nssm install $ServiceName $ExePath

# Directorio de trabajo = directorio del proyecto
nssm set $ServiceName AppDirectory $ProjectDir

# Logs stdout y stderr
nssm set $ServiceName AppStdout (Join-Path $LogDir "stdout.log")
nssm set $ServiceName AppStderr (Join-Path $LogDir "stderr.log")

# Rotar logs (cada 1MB, mantener 5 archivos)
nssm set $ServiceName AppStdoutCreationDisposition 4
nssm set $ServiceName AppStderrCreationDisposition 4
nssm set $ServiceName AppRotateFiles 1
nssm set $ServiceName AppRotateOnline 1
nssm set $ServiceName AppRotateBytes 1048576

# Reinicio automático ante fallo (esperar 5 segundos)
nssm set $ServiceName AppExit Default Restart
nssm set $ServiceName AppRestartDelay 5000

# Iniciar con Windows
nssm set $ServiceName Start SERVICE_AUTO_START

# Descripción del servicio
nssm set $ServiceName DisplayName "Go Gin Agent Orchestrator"
nssm set $ServiceName Description "Multi-agent AI orchestrator service"

# ── Cargar variables de .env ─────────────────────────────────
Write-Host "Cargando variables de entorno desde .env..."
$envVars = @()
Get-Content $EnvFile | ForEach-Object {
    $line = $_.Trim()
    # Ignorar comentarios y líneas vacías
    if ($line -and -not $line.StartsWith("#")) {
        # Separar KEY=VALUE y eliminar comentarios inline
        if ($line -match '^([^=]+)=(.*)$') {
            $key = $matches[1].Trim()
            $value = $matches[2].Trim()
            # Eliminar comentarios inline (# precedido de espacios)
            # Pero NO eliminar # dentro de comillas
            if ($value -match '^".*"$') {
                # Valor entre comillas: quitar las comillas
                $value = $value.Trim('"')
            } else {
                # Eliminar comentarios inline
                $value = ($value -replace '\s+#.*$', '').Trim()
            }
            if ($key -and $value) {
                $envVars += "$key=$value"
            }
        }
    }
}

if ($envVars.Count -gt 0) {
    $envString = $envVars -join "`n"
    nssm set $ServiceName AppEnvironmentExtra $envString
    Write-Host "[OK] $($envVars.Count) variables de entorno configuradas"
}

# ── Iniciar servicio ────────────────────────────────────────
Write-Host ""
Write-Host "Iniciando servicio..."
nssm start $ServiceName

Start-Sleep -Seconds 3

$svc = Get-Service -Name $ServiceName
if ($svc.Status -eq "Running") {
    Write-Host ""
    Write-Host "====================================" -ForegroundColor Green
    Write-Host " Servicio instalado y corriendo!" -ForegroundColor Green
    Write-Host "====================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "Comandos útiles:"
    Write-Host "  nssm status $ServiceName        — Ver estado"
    Write-Host "  nssm stop $ServiceName           — Detener"
    Write-Host "  nssm start $ServiceName          — Iniciar"
    Write-Host "  nssm restart $ServiceName        — Reiniciar"
    Write-Host "  nssm edit $ServiceName            — Editar configuración (GUI)"
    Write-Host "  nssm remove $ServiceName confirm — Desinstalar servicio"
    Write-Host ""
    Write-Host "Logs en: $LogDir"
} else {
    Write-Host ""
    Write-Host "El servicio no arrancó correctamente. Revisar logs en:" -ForegroundColor Red
    Write-Host "  $LogDir" -ForegroundColor Red  
    Write-Host ""
    Write-Host "También puedes ver los eventos con:"
    Write-Host "  Get-EventLog -LogName Application -Source nssm -Newest 10"
}
