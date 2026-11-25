/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Maximize2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'

export function ArtworkPreview(props: { html: string }) {
  const { t } = useTranslation()
  const [preview, setPreview] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const frameRef = useRef<HTMLIFrameElement>(null)
  useEffect(() => {
    if (!expanded) return
    function closeFromArtwork(event: MessageEvent<unknown>): void {
      if (
        event.source === frameRef.current?.contentWindow &&
        event.data === 'evaluation-preview:escape'
      ) {
        setExpanded(false)
      }
    }
    window.addEventListener('message', closeFromArtwork)
    return () => window.removeEventListener('message', closeFromArtwork)
  }, [expanded])
  const document = useMemo(() => {
    if (!preview) return ''
    // Keep the original document intact. Parse it only inside the opaque-origin
    // sandbox, with our CSP placed before any model-generated markup.
    return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; media-src data: blob:; connect-src 'none'; frame-src 'none'; worker-src 'none'; object-src 'none'; form-action 'none'; base-uri 'none'"><meta name="referrer" content="no-referrer"><script>document.addEventListener("keydown",function(event){if(event.key==="Escape")parent.postMessage("evaluation-preview:escape","*")})</script>${props.html}`
  }, [preview, props.html])

  const frame = (
    <iframe
      ref={frameRef}
      title={t('Generated artwork')}
      srcDoc={document}
      sandbox='allow-scripts'
      referrerPolicy='no-referrer'
      allow="camera 'none'; microphone 'none'; geolocation 'none'; clipboard-read 'none'; clipboard-write 'none'"
      className='h-full w-full rounded-lg border bg-white'
    />
  )

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <h3 className='font-semibold'>{t('Generated artwork')}</h3>
        <div className='flex flex-wrap gap-2'>
          <Button variant='outline' onClick={() => setPreview(!preview)}>
            {preview ? t('Close preview') : t('Load preview')}
          </Button>
          {preview && (
            <Button variant='outline' onClick={() => setExpanded(true)}>
              <Maximize2 className='size-4' aria-hidden />
              {t('Expand artwork')}
            </Button>
          )}
        </div>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Animations and controls run in an isolated preview. Site data, network requests and form submissions are restricted.'
        )}
      </p>
      {preview && (
        <div className='h-[clamp(32rem,78dvh,64rem)] w-full'>
          {!expanded && frame}
        </div>
      )}
      <Dialog
        open={expanded}
        onOpenChange={setExpanded}
        title={t('Generated artwork')}
        description={t(
          'Expanded preview. Press Escape to return to the report.'
        )}
        contentClassName='h-[calc(100dvh-2rem)] w-[calc(100vw-2rem)] max-w-none sm:max-w-none'
        contentHeight='100%'
        headerClassName='pe-8'
        bodyClassName='h-full'
      >
        {expanded && frame}
      </Dialog>
    </div>
  )
}
