import { Carousel, CarouselContent, CarouselItem, CarouselNext, CarouselPrevious } from "../../../shared/ui/carousel";
import { getImgproxyUrl } from "../../../shared/services/imgproxy";
import { toGatewayUrl } from "../../../shared/services/api";

type MediaImage = { url: string };

type Props = {
  images?: MediaImage[];
  fallbackUrl?: string;
  alt?: string;
  className?: string;
  imgClassName?: string;
};

export function PropertyMediaCarousel({ images, fallbackUrl, alt, className, imgClassName }: Props) {
  const items = (images || []).filter((i) => i?.url);
  const single = items[0]?.url || fallbackUrl;
  const hasCarousel = items.length > 1;

  if (!hasCarousel) {
    if (!single) return <div className={className} />;
    return (
      <img
        src={getImgproxyUrl(toGatewayUrl(single), { width: 900, height: 600 })}
        alt={alt || ""}
        className={imgClassName || className}
        loading="lazy"
        decoding="async"
      />
    );
  }

  return (
    <div className={className}>
      <Carousel opts={{ loop: true }} className="h-full">
        <CarouselContent className="h-full">
          {items.map((img, idx) => (
            <CarouselItem key={`${img.url}-${idx}`} className="h-full">
              <img
                src={getImgproxyUrl(toGatewayUrl(img.url), { width: 900, height: 600 })}
                alt={alt || ""}
                className={imgClassName || "w-full h-full object-cover"}
                loading="lazy"
                decoding="async"
              />
            </CarouselItem>
          ))}
        </CarouselContent>
        <CarouselPrevious className="left-4 top-1/2 -translate-y-1/2" />
        <CarouselNext className="right-4 top-1/2 -translate-y-1/2" />
      </Carousel>
    </div>
  );
}

