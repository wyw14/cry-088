<script setup lang="ts">defineProps<{label:string;value:string|number;tone?:'default'|'warning'}>()</script>
<template><el-card class="metric"><small>{{label}}</small><strong :class="tone">{{value}}</strong></el-card></template>
<style scoped>.metric{display:flex;flex-direction:column;gap:10px}.metric strong{font-size:24px;color:#17324d}.metric strong.warning{color:#b26a00}</style>
