import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';

const target = document.getElementById('app');
if (!target) throw new Error('The UI mount element is missing.');
export default mount(App, { target });
