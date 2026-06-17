import { useEffect, useRef, useState } from "react";
import { Mic, Square } from "lucide-react";
import { cn } from "@/lib/utils";

// Minimal type shim for the Web Speech API. The TS standard lib doesn't
// include these types because Firefox lacks support. We declare just enough
// to use the API safely; everything else stays `any`.
type SpeechRecognitionLike = {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  start: () => void;
  stop: () => void;
  abort: () => void;
  onresult: ((event: any) => void) | null;
  onerror: ((event: any) => void) | null;
  onend: (() => void) | null;
};

function getRecognitionCtor(): { new (): SpeechRecognitionLike } | null {
  if (typeof window === "undefined") return null;
  const w = window as any;
  return (w.SpeechRecognition || w.webkitSpeechRecognition) ?? null;
}

export function VoiceInput({
  onTranscript,
  disabled,
  className,
}: {
  onTranscript: (text: string, isFinal: true) => void;
  disabled?: boolean;
  className?: string;
}) {
  const [listening, setListening] = useState(false);
  const [supported] = useState<boolean>(() => getRecognitionCtor() !== null);
  const recRef = useRef<SpeechRecognitionLike | null>(null);

  // Best-effort language: prefer the document language, fall back to en-US.
  // Users can change <html lang="..."> if they want a different STT language.
  const lang =
    typeof document !== "undefined" && document.documentElement.lang
      ? document.documentElement.lang
      : "en-US";

  useEffect(() => {
    return () => {
      // Clean up on unmount: abort any in-flight recognition.
      if (recRef.current) {
        try {
          recRef.current.abort();
        } catch {
          // ignore
        }
        recRef.current = null;
      }
    };
  }, []);

  if (!supported) {
    // Silent degrade on Firefox / older browsers — render nothing rather
    // than show a button that errors on click.
    return null;
  }

  const start = () => {
    const Ctor = getRecognitionCtor();
    if (!Ctor) return;
    const rec = new Ctor();
    rec.lang = lang;
    rec.continuous = true;
    rec.interimResults = true;

    rec.onresult = (event: any) => {
      // Walk results, fire callback for any new final transcript.
      // Interim results are ignored — they would interfere with the user's
      // typed text. Only committed (final) speech is appended.
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const r = event.results[i];
        if (r.isFinal) {
          const transcript = r[0].transcript;
          if (transcript) onTranscript(transcript, true);
        }
      }
    };

    rec.onerror = (event: any) => {
      // Common: "no-speech" (user paused), "aborted" (user clicked stop).
      // Surface real errors via console but don't bother the user.
      if (event.error && event.error !== "no-speech" && event.error !== "aborted") {
        console.warn("voice input error", event.error);
      }
      setListening(false);
    };

    rec.onend = () => {
      // Recognition ends on its own after silence or on stop(). Update UI.
      setListening(false);
      recRef.current = null;
    };

    try {
      rec.start();
      recRef.current = rec;
      setListening(true);
    } catch (e) {
      console.warn("voice input start failed", e);
    }
  };

  const stop = () => {
    if (recRef.current) {
      try {
        recRef.current.stop();
      } catch {
        // ignore
      }
      recRef.current = null;
    }
    setListening(false);
  };

  const onClick = () => {
    if (disabled) return;
    if (listening) stop();
    else start();
  };

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={listening ? "Stop listening" : "Speak to text"}
      aria-pressed={listening}
      aria-label={listening ? "Stop voice input" : "Start voice input"}
      data-cursor="hover"
      className={cn(
        "inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border transition-all duration-200 ring-glow",
        listening
          ? "border-[var(--color-coral)]/40 bg-[var(--color-coral)]/15 text-[var(--color-coral)] animate-pulse"
          : "border-white/10 bg-white/5 text-[var(--color-ink-muted)] hover:bg-white/10 hover:text-[var(--color-ink)]",
        disabled && "opacity-40 cursor-not-allowed hover:bg-white/5",
        className,
      )}
    >
      {listening ? <Square className="h-5 w-5" /> : <Mic className="h-5 w-5" />}
    </button>
  );
}
