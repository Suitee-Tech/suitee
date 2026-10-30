import { describe, expect, it } from 'vitest'
import { detectVideoPlatform, extractVideoId, getVideoEmbedUrl } from './video-platform'

describe('video-platform', () => {
  it('detects youtube', () => {
    expect(detectVideoPlatform('https://www.youtube.com/watch?v=abc123')).toBe('youtube')
    expect(detectVideoPlatform('https://youtu.be/abc123')).toBe('youtube')
  })

  it('detects vimeo', () => {
    expect(detectVideoPlatform('https://vimeo.com/123456')).toBe('vimeo')
    expect(detectVideoPlatform('https://player.vimeo.com/video/123456')).toBe('vimeo')
  })

  it('extracts ids and embed urls', () => {
    expect(extractVideoId('https://www.youtube.com/watch?v=abc123').id).toBe('abc123')
    expect(extractVideoId('https://youtu.be/abc123').id).toBe('abc123')
    expect(extractVideoId('https://www.youtube.com/shorts/abc123').id).toBe('abc123')
    expect(extractVideoId('https://www.youtube.com/embed/abc123').id).toBe('abc123')
    expect(extractVideoId('https://vimeo.com/123456').id).toBe('123456')
    expect(extractVideoId('https://player.vimeo.com/video/123456').id).toBe('123456')

    expect(getVideoEmbedUrl({ url: 'https://www.youtube.com/watch?v=abc123' })).toBe(
      'https://www.youtube.com/embed/abc123',
    )
    expect(getVideoEmbedUrl({ url: 'https://vimeo.com/123456' })).toBe('https://player.vimeo.com/video/123456')
  })
})

