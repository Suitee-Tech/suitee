import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { ChevronLeft, ChevronRight, Expand, ImageOff } from "lucide-react";

import { Carousel, CarouselContent, CarouselItem, type CarouselApi } from "../../../shared/ui/carousel";
import { Dialog, DialogClose, DialogContent, DialogTitle } from "../../../shared/ui/dialog";
import { getImgproxyUrl } from "../../../shared/services/imgproxy";
import { toGatewayUrl } from "../../../shared/services/api";
import { cn } from "../../../shared/ui/utils";

export type PropertyGalleryImage = { url: string; description?: string };

type Props = {
  images?: PropertyGalleryImage[] | null;
  fallbackUrl?: string;
  alt?: string;
  /** Caixa da foto principal. As miniaturas ficam FORA dela (não são cortadas). */
  className?: string;
};

/** Distância a partir da qual um gesto é arraste (troca de foto) e não toque. */
const SWIPE_TOLERANCE = 8;

/** Índice da foto visível, acompanhando o Embla (arraste, seta ou teclado). */
function useCarouselIndex(api?: CarouselApi): number {
  const [index, setIndex] = useState(0);
  useEffect(() => {
    if (!api) return;
    const onSelect = () => setIndex(api.selectedScrollSnap());
    onSelect();
    api.on("select", onSelect);
    api.on("reInit", onSelect);
    return () => {
      api.off("select", onSelect);
      api.off("reInit", onSelect);
    };
  }, [api]);
  return index;
}

/**
 * Galeria do imóvel no portal do cliente.
 *
 * O cliente decide pelo que vê: foto grande com arraste, contador, legenda
 * cadastrada ("Fachada", "Sala de estar"), miniaturas para pular direto ao
 * ambiente e toque (ou Enter no botão "Ampliar") para ver em tela cheia, onde
 * arraste, setas e teclado continuam valendo. A foto é o principal ativo da
 * decisão: antes o detalhe caía num placeholder de ícone quando o imóvel não
 * vinha no formato esperado, e as miniaturas ficavam cortadas pelo
 * `overflow-hidden` da caixa que recebia a altura fixa.
 */
export function PropertyGallery({ images, fallbackUrl, alt, className }: Props) {
  const items = (images ?? []).filter((image) => image?.url);
  const heroUrl = items[0]?.url || fallbackUrl || "";
  const total = items.length;
  const hasCarousel = total > 1;

  const [api, setApi] = useState<CarouselApi>();
  const [viewerApi, setViewerApi] = useState<CarouselApi>();
  const [viewerOpen, setViewerOpen] = useState(false);
  const [startIndex, setStartIndex] = useState(0);
  const [loaded, setLoaded] = useState(false);
  const thumbsRef = useRef<Array<HTMLButtonElement | null>>([]);
  const pointerStart = useRef<{ x: number; y: number } | null>(null);

  const selected = useCarouselIndex(api);
  const viewerIndex = useCarouselIndex(viewerApi);

  // Mantém a miniatura ativa à vista (arraste, seta ou teclado).
  useEffect(() => {
    thumbsRef.current[selected]?.scrollIntoView({ behavior: "smooth", block: "nearest", inline: "center" });
  }, [selected]);

  const openViewer = () => {
    setStartIndex(selected);
    setViewerOpen(true);
  };

  // Um toque abre a tela cheia; um arraste troca de foto e não abre.
  const handlePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    pointerStart.current = { x: event.clientX, y: event.clientY };
  };
  const handlePointerUp = (event: ReactPointerEvent<HTMLDivElement>) => {
    const start = pointerStart.current;
    pointerStart.current = null;
    if (!start) return;
    if ((event.target as HTMLElement).closest("button")) return; // setas/ampliar têm ação própria
    if (Math.abs(event.clientX - start.x) > SWIPE_TOLERANCE) return;
    if (Math.abs(event.clientY - start.y) > SWIPE_TOLERANCE) return;
    openViewer();
  };

  const handleViewerOpenChange = (open: boolean) => {
    setViewerOpen(open);
    // Ao fechar, a galeria da página continua de onde o cliente parou.
    if (!open && hasCarousel) api?.scrollTo(viewerIndex);
  };

  if (!heroUrl && total === 0) {
    return (
      <div className={cn("flex flex-col items-center justify-center gap-2 bg-muted text-muted-foreground", className)}>
        <ImageOff className="w-8 h-8" aria-hidden="true" />
        <p className="text-xs">Fotos em preparação</p>
      </div>
    );
  }

  const imageUrl = (url: string, width: number, height: number) =>
    getImgproxyUrl(toGatewayUrl(url), { width, height });

  const caption = items[selected]?.description;
  const slideLabel = (index: number, description?: string) =>
    `Foto ${index + 1} de ${total}${description ? `: ${description}` : ""}`;

  const controls = (
    <>
      {hasCarousel && (
        <>
          <button
            type="button"
            onClick={() => api?.scrollPrev()}
            aria-label="Foto anterior"
            className="absolute left-1 top-1/2 z-10 hidden h-9 w-9 -translate-y-1/2 items-center justify-center rounded-full bg-foreground/50 text-background backdrop-blur-sm transition hover:bg-foreground/70 sm:flex"
          >
            <ChevronLeft className="h-5 w-5" aria-hidden="true" />
          </button>
          <button
            type="button"
            onClick={() => api?.scrollNext()}
            aria-label="Próxima foto"
            className="absolute right-1 top-1/2 z-10 hidden h-9 w-9 -translate-y-1/2 items-center justify-center rounded-full bg-foreground/50 text-background backdrop-blur-sm transition hover:bg-foreground/70 sm:flex"
          >
            <ChevronRight className="h-5 w-5" aria-hidden="true" />
          </button>
        </>
      )}
      <button
        type="button"
        onClick={openViewer}
        className="absolute right-3 top-3 z-10 flex items-center gap-1.5 rounded-full bg-foreground/60 px-2.5 py-1.5 text-caption font-semibold text-background backdrop-blur-sm transition hover:bg-foreground/80"
      >
        <Expand className="h-3.5 w-3.5" aria-hidden="true" />
        Ampliar
        <span className="sr-only">{`foto ${selected + 1} de ${total}`}</span>
      </button>
    </>
  );

  return (
    <div className="bg-muted">
      <div
        className={cn("relative overflow-hidden bg-muted", className)}
        onPointerDown={handlePointerDown}
        onPointerUp={handlePointerUp}
      >
        {!loaded && <span className="absolute inset-0 animate-pulse bg-foreground/5" aria-hidden="true" />}

        {hasCarousel ? (
          <Carousel opts={{ loop: true }} setApi={setApi} className="h-full">
            <CarouselContent className="h-full ml-0">
              {items.map((image, index) => (
                <CarouselItem
                  key={`${image.url}-${index}`}
                  className="h-full pl-0"
                  aria-label={slideLabel(index, image.description)}
                >
                  <img
                    src={imageUrl(image.url, 1200, 800)}
                    alt={image.description || alt || `Foto ${index + 1}`}
                    className="w-full h-full object-cover"
                    loading={index === 0 ? "eager" : "lazy"}
                    decoding="async"
                    draggable={false}
                    onLoad={index === 0 ? () => setLoaded(true) : undefined}
                    onError={index === 0 ? () => setLoaded(true) : undefined}
                  />
                </CarouselItem>
              ))}
            </CarouselContent>
            {controls}
          </Carousel>
        ) : (
          <div className="relative h-full">
            <img
              src={imageUrl(heroUrl, 1200, 800)}
              alt={items[0]?.description || alt || "Imóvel"}
              className="w-full h-full object-cover"
              loading="eager"
              decoding="async"
              draggable={false}
              onLoad={() => setLoaded(true)}
              onError={() => setLoaded(true)}
            />
            {controls}
          </div>
        )}

        {caption ? (
          <span className="pointer-events-none absolute bottom-3 left-3 rounded-full bg-foreground/70 px-3 py-1 text-caption font-semibold text-background backdrop-blur-sm">
            {caption}
          </span>
        ) : null}
        {hasCarousel && (
          <span
            className="pointer-events-none absolute bottom-3 right-3 rounded-full bg-foreground/70 px-2.5 py-1 text-caption font-bold text-background tabular-nums backdrop-blur-sm"
            aria-live="polite"
          >
            {selected + 1}/{total}
          </span>
        )}
      </div>

      {/* Miniaturas: pular direto para o ambiente que interessa. */}
      {hasCarousel && (
        <div className="mt-2 flex gap-2 overflow-x-auto px-1 pb-1">
          {items.map((image, index) => (
            <button
              key={`thumb-${image.url}-${index}`}
              ref={(node) => {
                thumbsRef.current[index] = node;
              }}
              type="button"
              onClick={() => api?.scrollTo(index)}
              aria-label={`Ver foto ${index + 1}${image.description ? `: ${image.description}` : ""}`}
              aria-current={index === selected}
              className={cn(
                "h-14 w-20 shrink-0 overflow-hidden rounded-lg border-2 transition-colors",
                index === selected ? "border-primary" : "border-transparent opacity-70",
              )}
            >
              <img
                src={imageUrl(image.url, 240, 168)}
                alt=""
                className="h-full w-full object-cover"
                loading="lazy"
                decoding="async"
                draggable={false}
              />
            </button>
          ))}
        </div>
      )}

      {/* Tela cheia: a foto é o que decide, então ela ocupa a tela. */}
      <Dialog open={viewerOpen} onOpenChange={handleViewerOpenChange}>
        <DialogContent
          showCloseButton={false}
          aria-describedby={undefined}
          // Ocupa a tela inteira: `inset-0` + altura de viewport dinâmica, sem a
          // animação de zoom/fade padrão (num visualizador de foto ela deixava a
          // página aparecendo em volta e por baixo durante a abertura).
          className="inset-0 top-0 left-0 h-[100dvh] w-screen max-w-none translate-x-0 translate-y-0 gap-0 rounded-none border-0 bg-black p-0 text-white data-[state=closed]:animate-none data-[state=open]:animate-none sm:max-w-none"
        >
          <DialogTitle className="sr-only">{`Fotos de ${alt || "imóvel"}`}</DialogTitle>
          <div className="flex h-full w-full flex-col">
            <div className="flex items-center justify-between px-4 py-3">
              <span className="text-caption font-semibold text-white/70 tabular-nums" aria-live="polite">
                {`${viewerIndex + 1}/${total}`}
              </span>
              <DialogClose className="rounded-full p-2 text-white/80 transition hover:bg-white/10 hover:text-white">
                <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
                  <path d="M6 6l12 12M18 6L6 18" strokeLinecap="round" />
                </svg>
                <span className="sr-only">Fechar</span>
              </DialogClose>
            </div>

            <Carousel opts={{ loop: true, startIndex }} setApi={setViewerApi} className="min-h-0 flex-1">
              <CarouselContent className="h-full ml-0">
                {items.map((image, index) => (
                  <CarouselItem
                    key={`viewer-${image.url}-${index}`}
                    className="flex h-full items-center justify-center pl-0"
                    aria-label={slideLabel(index, image.description)}
                  >
                    <img
                      src={imageUrl(image.url, 1600, 1200)}
                      alt={image.description || alt || `Foto ${index + 1}`}
                      className="max-h-full max-w-full object-contain"
                      loading="lazy"
                      decoding="async"
                      draggable={false}
                    />
                  </CarouselItem>
                ))}
              </CarouselContent>
              {hasCarousel && (
                <>
                  <button
                    type="button"
                    onClick={() => viewerApi?.scrollPrev()}
                    aria-label="Foto anterior"
                    className="absolute left-2 top-1/2 z-10 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/15 text-white transition hover:bg-white/25"
                  >
                    <ChevronLeft className="h-5 w-5" aria-hidden="true" />
                  </button>
                  <button
                    type="button"
                    onClick={() => viewerApi?.scrollNext()}
                    aria-label="Próxima foto"
                    className="absolute right-2 top-1/2 z-10 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/15 text-white transition hover:bg-white/25"
                  >
                    <ChevronRight className="h-5 w-5" aria-hidden="true" />
                  </button>
                </>
              )}
            </Carousel>

            <div className="flex flex-col items-center gap-3 px-6 pb-8 pt-3 text-center">
              {items[viewerIndex]?.description && (
                <p className="text-sm font-semibold text-white">{items[viewerIndex].description}</p>
              )}
              {hasCarousel && (
                <div className="flex items-center gap-1.5">
                  {items.map((image, index) => (
                    <button
                      key={`dot-${image.url}-${index}`}
                      type="button"
                      onClick={() => viewerApi?.scrollTo(index)}
                      aria-label={`Ir para foto ${index + 1}`}
                      aria-current={index === viewerIndex}
                      className={cn(
                        "h-1.5 rounded-full transition-all",
                        index === viewerIndex ? "w-6 bg-white" : "w-1.5 bg-white/40 hover:bg-white/70",
                      )}
                    />
                  ))}
                </div>
              )}
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
