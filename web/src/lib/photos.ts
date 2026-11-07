import { upload, V1 } from './api'

export interface Photo { id: number; w: number; h: number; url: string; thumb: string }

/** Shrink a phone photo before it goes up (turned the right way, at most 1920 px). */
async function shrink(file: File, side = 1920): Promise<File> {
  if (!file.type.startsWith('image/') || file.type === 'image/gif') return file
  try {
    const bmp = await createImageBitmap(file, { imageOrientation: 'from-image' })
    const k = Math.min(1, side / Math.max(bmp.width, bmp.height))
    const c = document.createElement('canvas')
    c.width = Math.round(bmp.width * k); c.height = Math.round(bmp.height * k)
    c.getContext('2d')!.drawImage(bmp, 0, 0, c.width, c.height)
    bmp.close()
    const blob = await new Promise<Blob | null>(res => c.toBlob(res, 'image/jpeg', 0.86))
    return blob ? new File([blob], file.name.replace(/\.\w+$/, '') + '.jpg', { type: 'image/jpeg' }) : file
  } catch { return file }
}

export async function uploadPhoto(groupId: number, file: File): Promise<Photo> {
  return upload<Photo>(`${V1}/groups/${groupId}/photos`, 'file', await shrink(file))
}

export function messagePhotos(data: Record<string, unknown>): Photo[] {
  const p = (data as { photos?: Photo[] }).photos
  return Array.isArray(p) ? p : []
}
