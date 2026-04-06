# ============================================================
# uninstall-service.ps1 — Desinstala el servicio go-gin-agent
# ============================================================
# Ejecutar como Administrador

$ServiceName = "go-gin-agent"

$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if (-not $existing) {
    Write-Host "El servicio '$ServiceName' no está instalado." -ForegroundColor Yellow
    exit 0
}

Write-Host "Deteniendo y removiendo servicio '$ServiceName'..."
nssm stop $ServiceName confirm
nssm remove $ServiceName confirm

Write-Host ""
Write-Host "Servicio removido exitosamente." -ForegroundColor Green
