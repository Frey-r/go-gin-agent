Arquitectura del Orquestador (Go + Gin)

El orquestador desarrollado en Go actúa como el middleware central de la plataforma. Su función no es generar el texto (de eso se encarga el LLM), sino administrar el ciclo de vida de la petición, la memoria, la ejecución de herramientas (Tools/MCP) y la entrega en tiempo real al frontend en Vue.
1. Patrón de Conexión: Server-Sent Events (SSE)

En lugar de usar WebSockets (que son bidireccionales y más pesados de mantener), la arquitectura utiliza SSE (Server-Sent Events).

    Naturaleza Unidireccional: El cliente (Vue) hace un POST estándar con su pregunta y abre una conexión HTTP persistente. Go toma esa conexión y comienza a "escupir" (stream) fragmentos de datos a medida que el LLM los genera.

    Implementación en Go: Se utiliza el http.Flusher integrado en Go/Gin. Por cada chunk o token recibido del modelo (OpenAI/Minimax), Go escribe en el buffer y ejecuta Flush() para empujarlo inmediatamente a la red.

    Bypass de Buffering: Para que el streaming funcione perfectamente a través de Cloudflare (y no entregue todo el texto de golpe al final), el orquestador inyecta los headers:

        Content-Type: text/event-stream

        Cache-Control: no-cache

        Connection: keep-alive

        X-Accel-Buffering: no (Clave para atravesar proxies inversos y túneles).

2. Manejo de Estados (State Management)

El orquestador en Go está diseñado para ser Stateless (sin estado) en la capa de red, lo que permite que sea altamente escalable o que se reinicie sin perder datos. El estado conversacional se maneja en la capa de persistencia (Base de Datos):

    Identificador de Sesión (Thread_ID): Cada petición que llega desde el frontend trae un Tenant_ID (la comunidad) y un Thread_ID (la conversación actual).

    Rehidratación de Contexto: Antes de llamar al LLM, Go consulta la base de datos (ej. tu SQLite con pgvector) usando el Thread_ID para recuperar la "Memoria a Corto Plazo" (los últimos X mensajes de la conversación).

    El Bucle de Razonamiento (Tool Calling): Aquí es donde Go brilla gestionando el estado interno de la petición:

        Go envía el historial + la nueva pregunta al LLM.

        Si el LLM decide usar una herramienta (ej. consultar saldo), devuelve un objeto JSON en lugar de texto. Go intercepta esto, pausa el streaming de texto al usuario (o envía un evento SSE tipo {"status": "Ejecutando integración..."}), y ejecuta el script de Python o el flujo MCP correspondiente.

        Go toma el resultado de esa herramienta, actualiza el estado temporal en memoria, y vuelve a llamar al LLM con la nueva información.

        Cuando el LLM finalmente responde con texto natural, Go reanuda el Flush() de los tokens hacia el usuario.

    Persistencia Final: Una vez que la conexión SSE se cierra (respuesta completada), Go guarda el nuevo intercambio (User -> Assistant) en la base de datos para futuras consultas.

3. Topología y Ruteo (Endpoints)

El servidor expone una API REST minimalista pero robusta:

    POST /api/v1/chat/stream: Endpoint principal que recibe el payload y devuelve el text/event-stream.

    POST /api/v1/webhooks: Para integraciones asíncronas de otros sistemas del cliente.

    GET /health: Endpoint crítico para que NinjaRMM o Coolify verifiquen el estado del servidor y el consumo de RAM.

4. Observabilidad y Facturación (El Hilo Asíncrono)

Para no añadir latencia al usuario, el cálculo de costos no bloquea el hilo principal.

    Goroutines para Telemetría: Una vez que se envía el último token al frontend por SSE, Go dispara una Goroutine (un hilo ligero en segundo plano).

    Langfuse: Esta Goroutine empaqueta la cantidad de tokens exactos de entrada y salida, el tiempo de latencia y el Tenant_ID, enviándolo de forma asíncrona a Langfuse. Esto es lo que permite facturar a fin de mes los excedentes de los planes "Bolsa de Consumo" sin ralentizar la experiencia del chat.