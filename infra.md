Arquitectura: Servicio Multiagente Síncrono

    [!info] Visión General
    Sistema de IA multiagéntico diseñado para baja latencia (síncrono), bajo costo operativo y alta escalabilidad. Utiliza un patrón de "Orquestador Ligero" delegando el razonamiento al LLM y la ejecución pesada a microservicios Serverless y procesos Offline.

1. Capa de Presentación (Edge)

    Stack: Vue 3 + Pinia + API de Composición.

    Hosting: Cloudflare Pages.

    Función: Interfaz de usuario reactiva. Mantiene estados explícitos (procesando, consultando, generando, finalizado) sin depender de polling.

    Conexión: Consume el streaming del backend a través de Server-Sent Events (SSE).

2. Cerebro Central (Orquestador Core)

    Stack: Binario compilado en Go + framework Gin.

    Infraestructura: VPS Bare-metal en Vultr (Datacenter: Santiago de Chile) para latencia local ínfima.

    Gestión de Procesos: systemd (mantiene el binario vivo y reinicia ante fallos).

    Proxy Inverso: Caddy o Nginx (gestión de puertos y certificados SSL automáticos).

    Estado y Memoria: SQLite local. Almacena el historial de conversación a corto plazo con latencia cero y sin costos de red.

    Rol: Es un orquestador "tonto" y extremadamente rápido. No toma decisiones de flujo; inyecta el contexto, llama a la API del modelo y despacha las herramientas usando Goroutines para no bloquear hilos.

3. Motor de Inferencia y Enrutamiento

    Stack: API de LLM (OpenAI / Anthropic / Gemini).

    Función: Actúa como el motor de razonamiento. Recibe el prompt y el contexto, decide de forma autónoma si tiene la información necesaria o si debe ejecutar un Tool Call (llamada a función). Redacta la respuesta final.

4. Brazos Ejecutores (Microservicios)

    [!success] Estrategia de Desacoplamiento
    El orquestador (Go) nunca ejecuta tareas pesadas ni manipula datos complejos. Todo el cómputo intensivo está aislado.

    Lambdas (AWS): Funciones Serverless en Python para tareas tradicionales (scraping, formateo de datos, integraciones de API). Costo por milisegundo de ejecución.

    Servidor MCP (Cloudflare Workers): Implementación del Model Context Protocol en el Edge. Actúa como el puente seguro y estandarizado para consultar la base de conocimiento corporativa.

5. Ingesta de Datos y Conocimiento (GraphRAG)

    [!warning] Patrón Offline y "Estrategia Discord"
    La ingesta de documentos está estrictamente separada del chat síncrono para proteger los recursos del servidor y la experiencia del usuario.

    Vía 1: Documentos Efímeros (Estrategia Discord): Archivos .txt subidos por el usuario vía HTTP POST. Go los lee en memoria, los inyecta como contexto (user_message) directo al LLM y los desecha. No contaminan la base global.

    Vía 2: Conocimiento Corporativo (GraphRAG):

        Motor de Ingesta: Máquina local. Procesa PDFs/Documentos en horas valle para evitar costos de nube. Extrae entidades y relaciones.

        Almacenamiento: Base de datos vectorial/grafo en la nube (ej. Supabase, Pinecone, Neo4j).

        Consumo: El servidor MCP en Cloudflare consulta esta base de datos solo cuando el LLM lo solicita.

6. Observabilidad y Facturación (Unit Economics)

    Stack: Langfuse (integrado vía SDK de Go).

    Función: Trazabilidad completa de la ejecución (Waterfall logging) y cálculo exacto de costos por token.

    Manejo de Trazas: Ejecutado en segundo plano mediante Goroutines concurrentes. Añade 0ms de latencia al usuario.

    Multitenancy: Inyección del tag tenant_id desde Go para separar costos por cliente B2B.

    Dashboard de Cliente: La UI en Vue consume la API REST de Langfuse a través del servidor Go para mostrar el gasto en tiempo real y la proyección a fin de mes de forma transparente.