# Docker Visual

Visualizador de contenedores y redes Docker, estilo Railway pero para imágenes locales.

## Stack

- **Backend:** Go + Gin + docker/client
- **Frontend:** Vue 3 + Pinia + TypeScript + Vite + vue-force-graph-2d

## Estructura

```
docker-visual/
├── backend/              # API Go + Gin
│   ├── cmd/server/       # Entry point
│   ├── internal/
│   │   ├── docker/       # Cliente Docker
│   │   ├── handlers/     # Handlers HTTP
│   │   └── models/       # Modelos de datos
│   └── go.mod
├── frontend/             # UI Vue
│   ├── src/
│   │   ├── components/   # Componentes Vue
│   │   ├── stores/       # Pinia stores
│   │   ├── types/        # Typescript types
│   │   └── App.vue
│   └── package.json
└── README.md
```

## Ejecutar

### Backend

```bash
cd backend
go mod tidy
go run cmd/server/main.go
```

El servidor corre en `:8080`. Requiere acceso al socket Docker (`/var/run/docker.sock`).

### Frontend

```bash
cd frontend
npm install
npm run dev
```

La UI corre en `http://localhost:5173` con proxy a `:8080`.

## API Endpoints

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/health` | Health check |
| GET | `/api/containers` | Listar contenedores |
| GET | `/api/containers/:id` | Detalle contenedor |
| POST | `/api/containers/:id/start` | Iniciar contenedor |
| POST | `/api/containers/:id/stop` | Detener contenedor |
| DELETE | `/api/containers/:id` | Eliminar contenedor |
| GET | `/api/networks` | Listar redes |
| GET | `/api/images` | Listar imágenes |
| GET | `/api/volumes` | Listar volúmenes |
| GET | `/api/graph` | Datos para grafo (containers + networks) |

## Vistas

- **Graph:** Visualización interactiva de containers y redes
- **Containers:** Lista con acciones start/stop/remove
- **Networks:** Lista de redes y sus containers conectados
- **Images:** Galería de imágenes locales
- **Volumes:** Lista de volúmenes
