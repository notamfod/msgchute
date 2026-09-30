<template>
  <div class="app-wrapper d-flex flex-column min-vh-100 bg-light">

    <!-- Navigation Bar -->
    <BNavbar variant="dark" v-b-color-mode="'dark'" class="shadow-sm">
      <BContainer class="d-flex justify-content-between align-items-center">
        <BNavbarBrand to="/" class="fw-bold m-0 text-white">MSGChute</BNavbarBrand>

        <BNavbarNav class="flex-row gap-3 align-items-center">
          <BNavItem to="/" active-class="active">Main</BNavItem>

          <!-- Correct Component: BNavItemDropdown -->
          <BNavItemDropdown
              text="Admin"
              :active="$route.path.startsWith('/admin')"
              active-class="active"
          >
            <BDropdownItem to="/admin/messages">Messages</BDropdownItem>
            <BDropdownItem to="/admin/templates">Templates</BDropdownItem>
            <BDropdownItem to="/admin/subscriptions">Подписки</BDropdownItem>
            <BDropdownItem to="/admin/worker/statuses">Worker Statuses</BDropdownItem>
          </BNavItemDropdown>

          <!-- Correct Component: BNavItemDropdown -->
          <BNavItemDropdown
              text="Public"
              :active="$route.path.startsWith('/send')"
              active-class="active"
          >
            <BDropdownItem to="/send">Send Message</BDropdownItem>
          </BNavItemDropdown>

          <BNavItem to="/config" active-class="active">Settings</BNavItem>
        </BNavbarNav>
      </BContainer>
    </BNavbar>

    <ConfigData />

    <!-- Core Content Canvas -->
    <main class="flex-grow-1 d-flex align-items-center py-5">
      <BContainer>
        <BRow justify-content="center" class="w-100 m-0">
          <BCol cols="12" md="12" lg="12" xl="12">
            <RouterView v-slot="{ Component }">
              <Transition name="fade" mode="out-in">
                <component :is="Component" />
              </Transition>
            </RouterView>
          </BCol>
        </BRow>
      </BContainer>
    </main>

  </div>
</template>

<script setup lang="ts">
import ConfigData from "@/components/ConfigData.vue";
</script>

<style scoped>
:deep(.nav-link) {
  color: rgba(255, 255, 255, 0.75) !important;
  font-weight: 500 !important;
  transition: color 0.15s ease-in-out;
}

:deep(.nav-link:hover),
:deep(.show > .nav-link) {
  color: #ffffff !important;
}

:deep(.nav-link.active) {
  color: #ffffff !important;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
