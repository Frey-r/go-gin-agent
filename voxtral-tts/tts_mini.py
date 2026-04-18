import torch
from transformers import pipeline, AutoTokenizer, AutoModelForCausalLM
import soundfile as sf
import io
import sys
import argparse
import os

# Load Voxtral Mini 3B TTS (quantized for Pi)
model_id = "mistralai/Voxtral-Mini-3B-2507"  # Adjust if TTS specific
device = "cuda" if torch.cuda.is_available() else "cpu"

pipe = pipeline("text-to-speech", model=model_id, device=device, torch_dtype=torch.float16)

def tts(text, output="output.wav", voice="default"):
    audio = pipe(text, voice=voice)
    sf.write(output, audio["audio"], audio["sampling_rate"])
    print(f"TTS generado: {output}")

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("text", type=str)
    parser.add_argument("--output", default="output.wav")
    args = parser.parse_args()
    tts(args.text, args.output)