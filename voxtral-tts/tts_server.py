from flask import Flask, request, send_file
from transformers import pipeline
import torch
import soundfile as sf
import io
import os

app = Flask(__name__)

# Voxtral TTS pipeline
model_id = "mistralai/Voxtral-4B-TTS-2603"  # Latest TTS
device = "cpu"
pipe = pipeline("text-to-speech", model=model_id, device=device)

@app.route('/tts', methods=['POST'])
def tts():
    text = request.json['text']
    audio = pipe(text)
    buffer = io.BytesIO()
    sf.write(buffer, audio['audio'], audio['sampling_rate'], format='wav')
    buffer.seek(0)
    return send_file(buffer, mimetype='audio/wav', as_attachment=True, download_name='voxtral.wav')

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8000)