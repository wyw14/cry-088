<script setup lang="ts">import { RouterView } from 'vue-router'</script>
<template><el-container class="shell"><el-header><strong>工时闭环</strong><span>项目投入与成本</span></el-header><el-main><RouterView /></el-main></el-container></template>
<style scoped>.shell{min-height:100vh;background:#f5f7fa}.el-header{display:flex;align-items:center;gap:24px;background:#17324d;color:#fff}.el-header span{opacity:.75}.el-main{max-width:1200px;width:100%;margin:0 auto}</style>
