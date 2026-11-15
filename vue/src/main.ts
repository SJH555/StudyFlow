import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createVuetify } from 'vuetify'
import {
  VAlert,
  VApp,
  VBtn,
  VBtnToggle,
  VCard,
  VCardTitle,
  VCardText,
  VChip,
  VColorPicker,
  VDatePicker,
  VDialog,
  VDivider,
  VIcon,
  VList,
  VListItem,
  VMenu,
  VNumberInput,
  VSelect,
  VSlider,
  VSnackbar,
  VSwitch,
  VTextField,
  VTextarea,
  VTooltip,
  VVirtualScroll,
} from 'vuetify/components'
import { mdi } from 'vuetify/iconsets/mdi'
import { zhHans } from 'vuetify/locale'
import App from './App.vue'
import '@mdi/font/css/materialdesignicons.css'
import 'vuetify/styles'
import './shared/designTokens.css'
import './style.css'

const vuetify = createVuetify({
  components: {
    VAlert,
    VApp,
    VBtn,
    VBtnToggle,
    VCard,
    VCardTitle,
    VCardText,
    VChip,
    VColorPicker,
    VDatePicker,
    VDialog,
    VDivider,
    VIcon,
    VList,
    VListItem,
    VMenu,
    VNumberInput,
    VSelect,
    VSlider,
    VSnackbar,
    VSwitch,
    VTextField,
    VTextarea,
    VTooltip,
    VVirtualScroll,
  },
  locale: { locale: 'zhHans', messages: { zhHans } },
  icons: { defaultSet: 'mdi', sets: { mdi } },
  theme: {
    defaultTheme: 'studyflow',
    themes: {
      studyflow: {
        dark: false,
        colors: {
          primary: '#2932e1',
          surface: '#fbfbfc',
          background: '#ffffff',
          'on-surface': '#282821',
          outline: '#e8ebf2',
        },
      },
      studyflowReview: {
        dark: false,
        colors: {
          primary: '#2863f4',
          success: '#169d62',
          surface: '#ffffff',
          background: '#f6f8ff',
          'on-surface': '#202b40',
          outline: '#dce4f6',
        },
      },
    },
  },
})

createApp(App).use(createPinia()).use(vuetify).mount('#app')
