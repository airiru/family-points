import { createApp } from 'vue';
import App from './App.vue';
import './style.css';

const app = createApp(App);
app.config.errorHandler = err => {
  window.__errs = window.__errs || [];
  window.__errs.push(String((err && err.stack) || err));
  console.error(err);
};
app.mount('#app');
