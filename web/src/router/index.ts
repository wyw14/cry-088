import { createRouter, createWebHistory } from 'vue-router'
import Dashboard from '../pages/Dashboard.vue'
import Calendar from '../pages/Calendar.vue'
export default createRouter({ history: createWebHistory(), routes: [{ path: '/', component: Dashboard }, { path: '/calendar', component: Calendar }] })
