// Mejoras opcionales de la interfaz. Sin dependencias y sin código en línea (la CSP solo admite
// scripts del mismo origen). Si este archivo no carga, todo sigue funcionando con HTML puro.
(() => {
    'use strict';

    const MAX_BYTES = 10 * 1024 * 1024;

    // 1. Fechas en la hora local del navegador. El HTML trae la fecha en UTC como respaldo.
    document.querySelectorAll('time[data-local]').forEach((el) => {
        const d = new Date(el.getAttribute('datetime'));
        if (!Number.isNaN(d.getTime())) {
            el.textContent = d.toLocaleString('es-AR', { dateStyle: 'medium', timeStyle: 'short' });
        }
    });

    // 2. Formularios largos: avisan que están trabajando y evitan el doble envío.
    document.querySelectorAll('form[data-busy]').forEach((form) => {
        const input = form.querySelector('input[type="file"]');
        const errorBox = form.querySelector('[data-file-error]');

        const validate = () => {
            if (!input) return true;
            const file = input.files && input.files[0];
            let msg = '';
            if (file) {
                if (file.size === 0) msg = 'El archivo está vacío.';
                else if (file.size > MAX_BYTES) msg = 'El archivo supera los 10 MiB.';
                else if (!/\.pdf$/i.test(file.name) && file.type !== 'application/pdf') msg = 'El archivo no parece un PDF.';
            }
            input.setCustomValidity(msg);
            if (errorBox) {
                errorBox.textContent = msg;
                errorBox.hidden = msg === '';
            }
            return msg === '';
        };

        if (input) input.addEventListener('change', validate);

        form.addEventListener('submit', (event) => {
            if (!validate()) {
                event.preventDefault();
                return;
            }
            const button = form.querySelector('button[type="submit"]');
            if (button && button.dataset.busyText) {
                button.disabled = true;
                button.textContent = button.dataset.busyText;
            }
            const status = form.querySelector('[data-busy-status]');
            if (status) status.hidden = false;
        });
    });

    // 3. Botones de copiar. Ocultos por defecto: sin JavaScript no tendrían efecto.
    document.querySelectorAll('[data-copy]').forEach((button) => {
        button.hidden = false;
        const label = button.textContent;
        button.addEventListener('click', async () => {
            const target = document.querySelector(button.dataset.copy);
            if (!target) return;
            const text = target.textContent;
            try {
                await navigator.clipboard.writeText(text);
            } catch (_) {
                const range = document.createRange();
                range.selectNodeContents(target);
                const selection = window.getSelection();
                selection.removeAllRanges();
                selection.addRange(range);
                document.execCommand('copy');
                selection.removeAllRanges();
            }
            button.textContent = 'Copiado';
            setTimeout(() => { button.textContent = label; }, 1500);
        });
    });
})();
