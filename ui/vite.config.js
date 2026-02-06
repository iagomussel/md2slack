import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
    plugins: [tailwindcss(), sveltekit()],
    server: {
        proxy: {
            '/state': 'http://localhost:8088',
            '/tasks': 'http://localhost:8088',
            '/api': 'http://localhost:8088',
            '/settings': 'http://localhost:8088',
            '/run': 'http://localhost:8088',
            '/send': 'http://localhost:8088',
            '/action': 'http://localhost:8088',
            '/scan-users': 'http://localhost:8088'
        }
    }
});
