// Photos are shrunk on the device before upload, the way chat apps do: upright, at most
// 2048px on the long side, high-quality JPEG. Sharp at full screen on any phone or
// laptop, softer only when zoomed in, and a 4–8 MB camera photo uploads as a few
// hundred KB. The server re-checks size and orientation either way.

const MAX_EDGE = 2048
const QUALITY = 0.9

export async function shrinkImage(file: File): Promise<Blob> {
  if (!file.type.startsWith('image/') || file.type === 'image/gif' || typeof createImageBitmap !== 'function') return file
  let bmp: ImageBitmap
  try {
    bmp = await createImageBitmap(file, { imageOrientation: 'from-image' }) // apply the camera's rotation
  } catch {
    return file // a format this browser can't read; let the server try
  }
  try {
    const scale = Math.min(1, MAX_EDGE / Math.max(bmp.width, bmp.height))
    const w = Math.max(1, Math.round(bmp.width * scale))
    const h = Math.max(1, Math.round(bmp.height * scale))
    const canvas = document.createElement('canvas')
    canvas.width = w
    canvas.height = h
    const ctx = canvas.getContext('2d')
    if (!ctx) return file
    ctx.fillStyle = '#fff' // transparent PNGs get a white background, not black
    ctx.fillRect(0, 0, w, h)
    ctx.imageSmoothingEnabled = true
    ctx.imageSmoothingQuality = 'high'
    ctx.drawImage(bmp, 0, 0, w, h)
    const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/jpeg', QUALITY))
    return blob ?? file
  } finally {
    bmp.close()
  }
}
