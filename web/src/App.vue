<template>
  <el-config-provider :locale="elementPlusLocale">
    <router-view />
  </el-config-provider>
</template>

<script setup lang="ts">
import { elementPlusLocale } from "./lib/uiLocale";
</script>
