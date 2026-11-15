<script setup lang="ts">
import { storeToRefs } from "pinia";
import { useReviewStore } from "../../stores/review";
import { fmt, formatDateChinese, reviewGap } from "../../shared/format";
const {
  reviewPending,
  reviewDraft,
  reviewSaving,
  reviewBulkOffer,
  reviewBulkChoice,
  reviewBulkReason,
} = storeToRefs(useReviewStore());
const { submitReview, submitBulkReview } = useReviewStore();
</script>

<template>
  <template v-if="reviewPending"
    ><VDialog
      :model-value="true"
      persistent
      max-width="560"
      :z-index="3000"
      aria-labelledby="daily-review-title"
    >
      <VCard class="daily-review-dialog" rounded="xl"
        ><VCardTitle class="daily-review-dialog-title"
          ><VIcon icon="mdi-chart-timeline-variant" color="primary" /><span
            ><small>{{ formatDateChinese(reviewPending.day) }} · 进度追因</small
            ><strong id="daily-review-title">{{
              reviewBulkChoice === "ask" ? "连续未学习" : "记录未达标原因"
            }}</strong></span
          ></VCardTitle
        ><VCardText class="daily-review-dialog-content">
          <template v-if="reviewBulkOffer && reviewBulkChoice === 'ask'"
            ><p class="daily-review-intro">
              检测到从 {{ formatDateChinese(reviewBulkOffer.startDay) }} 至
              {{ formatDateChinese(reviewBulkOffer.endDay) }} 连续
              {{ reviewBulkOffer.daysWithoutStudy }} 天没有学习记录，其中
              {{ reviewBulkOffer.pendingCount }}
              天需要填写总结。是否统一填写原因？
            </p>
            <div class="daily-review-dialog-actions">
              <VBtn
                variant="text"
                rounded="lg"
                @click="reviewBulkChoice = 'individual'"
                >逐天填写</VBtn
              ><VBtn
                variant="tonal"
                color="primary"
                rounded="lg"
                @click="reviewBulkChoice = 'unified'"
                >统一填写</VBtn
              >
            </div></template
          >
          <template
            v-else-if="reviewBulkOffer && reviewBulkChoice === 'unified'"
            ><p class="daily-review-intro">
              一条原因将用于这
              {{ reviewBulkOffer.pendingCount }} 天的未达标总结。
            </p>
            <VTextarea
              v-model="reviewBulkReason"
              label="统一原因"
              placeholder="请填写这段时间没有学习的原因"
              variant="outlined"
              density="comfortable"
              rows="4"
              maxlength="1000"
              hide-details="auto"
            />
            <div class="daily-review-dialog-actions">
              <VBtn
                variant="text"
                rounded="lg"
                @click="reviewBulkChoice = 'ask'"
                >返回</VBtn
              ><VBtn
                variant="tonal"
                color="primary"
                rounded="lg"
                :loading="reviewSaving"
                @click="submitBulkReview"
                >提交统一原因</VBtn
              >
            </div></template
          >
          <template v-else
            ><p
              class="daily-review-intro"
              v-if="reviewPending!.mode === 'total'"
            >
              计划学习 {{ fmt(reviewPending!.plannedSeconds) }}，实际学习
              {{ fmt(reviewPending!.actualSeconds) }}，距离
              {{ reviewPending!.threshold }}% 的目标还差
              {{
                reviewGap(
                  reviewPending!.plannedSeconds,
                  reviewPending!.actualSeconds,
                  reviewPending!.threshold,
                )
              }}。请填写理由后继续。
            </p>
            <p class="daily-review-intro" v-else>
              当天计划学习 {{ fmt(reviewPending!.plannedSeconds) }}，实际学习
              {{ fmt(reviewPending!.actualSeconds) }}。以下项目未达到
              {{ reviewPending!.threshold }}% 的目标，请分别填写理由。
            </p>
            <div class="daily-review-dialog-items">
              <div v-for="item in reviewPending!.items" :key="item.subjectId">
                <strong>{{
                  reviewPending!.mode === "project"
                    ? item.subjectName
                    : "全天学习"
                }}</strong
                ><small
                  >计划 {{ fmt(item.plannedSeconds) }} · 实际
                  {{ fmt(item.actualSeconds) }} · 距目标还差
                  {{
                    reviewGap(
                      item.plannedSeconds,
                      item.actualSeconds,
                      reviewPending!.threshold,
                    )
                  }}</small
                ><VTextarea
                  v-model="reviewDraft[item.subjectId]"
                  :aria-label="
                    (reviewPending!.mode === 'project'
                      ? item.subjectName
                      : '全天学习') + '未达标原因'
                  "
                  placeholder="请填写未达标的原因"
                  variant="outlined"
                  density="comfortable"
                  rows="3"
                  maxlength="1000"
                  hide-details="auto"
                />
              </div>
            </div>
            <div class="daily-review-dialog-actions">
              <VBtn
                variant="tonal"
                color="primary"
                rounded="lg"
                :loading="reviewSaving"
                @click="submitReview"
                >提交总结</VBtn
              >
            </div></template
          >
        </VCardText></VCard
      >
    </VDialog>
  </template>
</template>

<style scoped>
.daily-review-intro {
  font-size: 14px;
  line-height: 1.7;
  color: var(--ink);
  margin: 18px 0;
}
.daily-review-dialog {
  display: flex;
  flex-direction: column;
  max-height: min(85vh, 760px);
  background: var(--surface-card);
  color: var(--ink);
}
.daily-review-dialog-title {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 22px 24px 12px;
  white-space: normal;
}
.daily-review-dialog-title > span {
  display: grid;
  gap: 3px;
}
.daily-review-dialog-title small {
  font-size: 12px;
  color: var(--muted);
  font-weight: 500;
}
.daily-review-dialog-title strong {
  font-size: 20px;
  line-height: 1.3;
}
.daily-review-dialog-content {
  overflow-y: auto;
  padding: 8px 24px 22px;
}
.daily-review-dialog-content .daily-review-intro {
  margin: 0 0 18px;
}
.daily-review-dialog-items {
  display: grid;
  gap: 18px;
}
.daily-review-dialog-items > div {
  display: grid;
  gap: 7px;
}
.daily-review-dialog-items strong {
  font-size: 14px;
}
.daily-review-dialog-items small {
  font-size: 12px;
  color: var(--muted);
}
.daily-review-dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 20px;
}
</style>
